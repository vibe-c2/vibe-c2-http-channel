package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	coreCache "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/cache"
	coreErrors "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/errors"
	coreMatcher "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/matcher"
	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	coreResolver "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/resolver"
	coreRuntime "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/runtime"
	coreSync "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/syncclient"
	coreTransform "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/transform"
)

type SyncResponse struct {
	ID            string `json:"id"`
	EncryptedData string `json:"encrypted_data"`
}

type envelope struct{ data map[string]string }

func (e *envelope) SourceKey() string { return "http" }
func (e *envelope) GetField(location, key string) (string, bool) {
	v, ok := e.data[location+"."+key]
	return v, ok
}
func (e *envelope) SetField(location, key, value string) { e.data[location+"."+key] = value }

func requestInput(r *http.Request, body map[string]any, rawBody string) coreResolver.Input {
	if body == nil {
		body = map[string]any{}
	}
	body[""] = rawBody
	body["raw"] = rawBody
	in := coreResolver.Input{Body: body, Headers: map[string]string{}, Query: map[string]string{}, Cookies: map[string]string{}}
	for k, v := range r.Header {
		if len(v) > 0 {
			in.Headers[k] = v[0]
			in.Headers[strings.ToLower(k)] = v[0]
		}
	}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			in.Query[k] = v[0]
		}
	}
	for _, c := range r.Cookies() {
		in.Cookies[c.Name] = c.Value
	}
	return in
}

func detectHint(input coreResolver.Input, profiles []coreProfile.Profile) string {
	for _, ref := range []string{"query:profile_id", "header:X-Profile-ID", "cookie:profile_id", "body:profile_id"} {
		if v, ok, _ := coreResolver.Resolve(ref, input); ok {
			return v
		}
	}
	for _, p := range profiles {
		if p.Mapping.ProfileID == nil {
			continue
		}
		if v, ok, _ := coreResolver.Resolve(p.Mapping.ProfileID.Ref(), input); ok {
			return v
		}
	}
	return ""
}

func resolveCombined(input coreResolver.Input, cf coreProfile.CombinedField) (string, error) {
	raw, ok, err := coreResolver.Resolve(cf.Ref(), input)
	if err != nil || !ok {
		if err != nil {
			return "", err
		}
		return "", coreErrors.New(coreErrors.CodeInvalidInput, "missing combined field: "+cf.Ref())
	}
	v, err := coreTransform.ApplyDecode(raw, cf.Transform)
	if err != nil {
		return "", err
	}
	return v, nil
}

func resolveMapped(input coreResolver.Input, mf coreProfile.MapField) (string, error) {
	if strings.EqualFold(strings.TrimSpace(mf.Target.Location), "body") && strings.TrimSpace(mf.Target.Key) == "" {
		raw, _ := input.Body[""].(string)
		v, err := coreTransform.ApplyDecode(raw, mf.Transform)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(v) == "" {
			return "", coreErrors.New(coreErrors.CodeInvalidInput, "missing mapped body payload")
		}
		return v, nil
	}
	raw, ok, err := coreResolver.Resolve(mf.Ref(), input)
	if err != nil || !ok {
		if err != nil {
			return "", err
		}
		return "", coreErrors.New(coreErrors.CodeInvalidInput, "missing mapped field: "+mf.Ref())
	}
	v, err := coreTransform.ApplyDecode(raw, mf.Transform)
	if err != nil {
		return "", err
	}
	return v, nil
}

type profilesProvider interface{ Profiles() []coreProfile.Profile }

type staticProfiles []coreProfile.Profile

func (s staticProfiles) Profiles() []coreProfile.Profile {
	return append([]coreProfile.Profile(nil), s...)
}

func New(addr, channelID, c2SyncBaseURL string, profiles []coreProfile.Profile) *http.Server {
	return NewWithProvider(addr, channelID, c2SyncBaseURL, staticProfiles(profiles))
}

func NewWithProvider(addr, channelID, c2SyncBaseURL string, provider profilesProvider) *http.Server {
	runtime := coreRuntime.New(coreSync.NewHTTPClient(c2SyncBaseURL, nil))
	matcher := coreMatcher.New()
	affinity := coreCache.NewAffinity(10 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rawBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		rawBody := string(rawBytes)
		var body map[string]any
		_ = json.Unmarshal(rawBytes, &body)
		input := requestInput(r, body, rawBody)

		profiles := provider.Profiles()
		hint := detectHint(input, profiles)
		if hint == "" {
			if p, ok := affinity.Get(r.RemoteAddr); ok {
				hint = p
			}
		}

		candidates := matcher.EnabledOrdered(profiles)
		if hint != "" {
			if res, err := matcher.Resolve(r.Context(), hint, profiles); err == nil {
				candidates = []coreProfile.Profile{res.Profile}
			} else if coreErrors.Code(err) == coreErrors.CodeProfileAmbiguous {
				http.Error(w, "profile resolution failed: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		var lastErr error
		for _, p := range candidates {
			pRun := p
			var id, encIn string
			if p.Mapping.CombinedIn != nil {
				raw, err := resolveCombined(input, *p.Mapping.CombinedIn)
				if err != nil {
					lastErr = err
					continue
				}
				sep := p.Mapping.CombinedIn.Separator
				if sep == "" {
					sep = "+"
				}
				parts := strings.SplitN(raw, sep, 2)
				if len(parts) != 2 {
					lastErr = coreErrors.New(coreErrors.CodeInvalidInput, "combined_in payload format mismatch")
					continue
				}
				id, encIn = parts[0], parts[1]
				pRun.Mapping.ID = coreProfile.MapField{Target: coreProfile.Target{Location: "mapping", Key: "id"}}
				pRun.Mapping.EncryptedDataIn = coreProfile.MapField{Target: coreProfile.Target{Location: "mapping", Key: "encrypted_data_in"}}
				pRun.Mapping.EncryptedDataOut = coreProfile.MapField{Target: coreProfile.Target{Location: "mapping", Key: "encrypted_data_out"}}
			} else {
				var err error
				id, err = resolveMapped(input, p.Mapping.ID)
				if err != nil {
					lastErr = err
					continue
				}
				encIn, err = resolveMapped(input, p.Mapping.EncryptedDataIn)
				if err != nil {
					lastErr = err
					continue
				}
			}

			env := &envelope{data: map[string]string{}}
			env.SetField("mapping", pRun.Mapping.ID.Target.Key, id)
			env.SetField("mapping", pRun.Mapping.EncryptedDataIn.Target.Key, encIn)
			if hint != "" && pRun.Mapping.ProfileID != nil {
				env.SetField("mapping", pRun.Mapping.ProfileID.Target.Key, hint)
			}

			outCanonical, err := runtime.HandleWithProfile(r.Context(), env, channelID, pRun)
			if err != nil {
				http.Error(w, "sync failed: "+err.Error(), http.StatusBadGateway)
				return
			}

			outID, err := coreTransform.ApplyEncode(outCanonical.ID, p.Mapping.ID.Transform)
			if err != nil {
				http.Error(w, "id encode failed", http.StatusBadRequest)
				return
			}
			outEnc, err := coreTransform.ApplyEncode(outCanonical.EncryptedData, p.Mapping.EncryptedDataOut.Transform)
			if err != nil {
				http.Error(w, "encrypted_data encode failed", http.StatusBadRequest)
				return
			}
			affinity.Set(r.RemoteAddr, p.ProfileID)

			if p.Mapping.CombinedOut != nil {
				sep := p.Mapping.CombinedOut.Separator
				if sep == "" {
					sep = "+"
				}
				combined, err := coreTransform.ApplyEncode(outCanonical.ID+sep+outCanonical.EncryptedData, p.Mapping.CombinedOut.Transform)
				if err != nil {
					http.Error(w, "combined_out encode failed", http.StatusBadRequest)
					return
				}
				if strings.EqualFold(strings.TrimSpace(p.Mapping.CombinedOut.Target.Location), "body") && strings.TrimSpace(p.Mapping.CombinedOut.Target.Key) == "" {
					w.Header().Set("Content-Type", "text/plain")
					_, _ = w.Write([]byte(combined))
					return
				}
			}

			if strings.EqualFold(strings.TrimSpace(p.Mapping.ID.Target.Location), "cookie") && strings.TrimSpace(p.Mapping.ID.Target.Key) != "" {
				http.SetCookie(w, &http.Cookie{Name: p.Mapping.ID.Target.Key, Value: outID, Path: "/"})
			}

			if strings.EqualFold(strings.TrimSpace(p.Mapping.EncryptedDataOut.Target.Location), "body") && strings.TrimSpace(p.Mapping.EncryptedDataOut.Target.Key) == "" {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(outEnc))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(SyncResponse{ID: outID, EncryptedData: outEnc})
			return
		}

		http.Error(w, "profile resolution failed: unmatched payload", http.StatusBadRequest)
		_ = lastErr
	})

	return &http.Server{Addr: addr, Handler: mux}
}

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

func New(addr, channelID, c2SyncBaseURL string, profiles []coreProfile.Profile) *http.Server {
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
			id, err := resolveMapped(input, p.Mapping.ID)
			if err != nil {
				lastErr = err
				continue
			}
			encIn, err := resolveMapped(input, p.Mapping.EncryptedDataIn)
			if err != nil {
				lastErr = err
				continue
			}

			env := &envelope{data: map[string]string{}}
			env.SetField("mapping", p.Mapping.ID.Target.Key, id)
			env.SetField("mapping", p.Mapping.EncryptedDataIn.Target.Key, encIn)
			if hint != "" && p.Mapping.ProfileID != nil {
				env.SetField("mapping", p.Mapping.ProfileID.Target.Key, hint)
			}

			outCanonical, err := runtime.HandleWithProfile(r.Context(), env, channelID, p)
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

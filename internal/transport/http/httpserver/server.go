package httpserver

import (
	"encoding/json"
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

type parsedMapRef struct {
	Ref   string
	Steps []coreTransform.Spec
}

func parseMapRef(raw string) parsedMapRef {
	parts := strings.Split(raw, "|")
	out := parsedMapRef{Ref: strings.TrimSpace(parts[0])}
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if p != "" {
			out.Steps = append(out.Steps, coreTransform.Spec{Type: p})
		}
	}
	return out
}

func detectHint(r *http.Request, body map[string]any) string {
	in := coreResolver.Input{Body: body, Headers: map[string]string{}, Query: map[string]string{}, Cookies: map[string]string{}}
	for k, v := range r.Header {
		if len(v) > 0 {
			in.Headers[k] = v[0]
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
	for _, ref := range []string{"query:profile_id", "header:X-Profile-ID", "cookie:profile_id", "body:profile_id"} {
		if v, ok, _ := coreResolver.Resolve(ref, in); ok {
			return v
		}
	}
	return ""
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

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		hint := detectHint(r, body)
		if hint == "" {
			if p, ok := affinity.Get(r.RemoteAddr); ok {
				hint = p
			}
		}
		resolution, err := matcher.Resolve(r.Context(), hint, profiles)
		if err != nil {
			if code := coreErrors.Code(err); code == coreErrors.CodeProfileAmbiguous || code == coreErrors.CodeProfileNotFound {
				http.Error(w, "profile resolution failed: "+err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "profile resolution failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		input := coreResolver.Input{Body: body, Headers: map[string]string{}, Query: map[string]string{}, Cookies: map[string]string{}}
		for k, v := range r.Header {
			if len(v) > 0 {
				input.Headers[k] = v[0]
			}
		}
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				input.Query[k] = v[0]
			}
		}
		for _, c := range r.Cookies() {
			input.Cookies[c.Name] = c.Value
		}

		idMap := parseMapRef(resolution.Profile.Mapping.ID)
		encMap := parseMapRef(resolution.Profile.Mapping.EncryptedData)
		idRaw, ok, err := coreResolver.Resolve(idMap.Ref, input)
		if err != nil || !ok {
			http.Error(w, "id not found by profile mapping", http.StatusBadRequest)
			return
		}
		encRaw, ok, err := coreResolver.Resolve(encMap.Ref, input)
		if err != nil || !ok {
			http.Error(w, "encrypted_data not found by profile mapping", http.StatusBadRequest)
			return
		}
		id, err := coreTransform.ApplyDecode(idRaw, idMap.Steps)
		if err != nil {
			http.Error(w, "id decode failed", http.StatusBadRequest)
			return
		}
		enc, err := coreTransform.ApplyDecode(encRaw, encMap.Steps)
		if err != nil {
			http.Error(w, "encrypted_data decode failed", http.StatusBadRequest)
			return
		}

		p := resolution.Profile
		p.Mapping.ID = idMap.Ref
		p.Mapping.EncryptedData = encMap.Ref
		if p.Mapping.ProfileID != "" {
			p.Mapping.ProfileID = parseMapRef(p.Mapping.ProfileID).Ref
		}

		env := &envelope{data: map[string]string{}}
		env.SetField("mapping", p.Mapping.ID, id)
		env.SetField("mapping", p.Mapping.EncryptedData, enc)
		if hint != "" && p.Mapping.ProfileID != "" {
			env.SetField("mapping", p.Mapping.ProfileID, hint)
		}

		outCanonical, err := runtime.HandleWithProfile(r.Context(), env, channelID, p)
		if err != nil {
			http.Error(w, "sync failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		outID, _ := coreTransform.ApplyEncode(outCanonical.ID, idMap.Steps)
		outEnc, _ := coreTransform.ApplyEncode(outCanonical.EncryptedData, encMap.Steps)
		affinity.Set(r.RemoteAddr, p.ProfileID)

		out := SyncResponse{ID: outID, EncryptedData: outEnc}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	return &http.Server{Addr: addr, Handler: mux}
}

package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	coreErrors "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/errors"
	coreMatcher "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/matcher"
	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	coreRuntime "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/runtime"
	coreSync "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/syncclient"
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

func splitRef(ref string) (location, key string) {
	parts := strings.SplitN(strings.TrimSpace(ref), ":", 2)
	if len(parts) == 2 {
		return strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
	}
	return "body", strings.TrimSpace(ref)
}

func valueFromRequest(r *http.Request, body map[string]any, ref string) (string, bool) {
	loc, key := splitRef(ref)
	switch loc {
	case "header":
		v := strings.TrimSpace(r.Header.Get(key))
		return v, v != ""
	case "query":
		v := strings.TrimSpace(r.URL.Query().Get(key))
		return v, v != ""
	case "cookie":
		c, err := r.Cookie(key)
		if err != nil {
			return "", false
		}
		v := strings.TrimSpace(c.Value)
		return v, v != ""
	case "body":
		if raw, ok := body[key]; ok {
			if s, ok := raw.(string); ok {
				s = strings.TrimSpace(s)
				return s, s != ""
			}
		}
		return "", false
	default:
		return "", false
	}
}

func detectHint(r *http.Request, body map[string]any) string {
	for _, ref := range []string{"query:profile_id", "header:X-Profile-ID", "cookie:profile_id", "body:profile_id"} {
		if v, ok := valueFromRequest(r, body, ref); ok {
			return v
		}
	}
	return ""
}

func New(addr, channelID, c2SyncBaseURL string, profiles []coreProfile.Profile) *http.Server {
	runtime := coreRuntime.New(coreSync.NewHTTPClient(c2SyncBaseURL, nil))
	matcher := coreMatcher.New()

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
		resolution, err := matcher.Resolve(r.Context(), hint, profiles)
		if err != nil {
			if code := coreErrors.Code(err); code == coreErrors.CodeProfileAmbiguous || code == coreErrors.CodeProfileNotFound {
				http.Error(w, "profile resolution failed: "+err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "profile resolution failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		id, ok := valueFromRequest(r, body, resolution.Profile.Mapping.ID)
		if !ok {
			http.Error(w, "id not found by profile mapping", http.StatusBadRequest)
			return
		}
		enc, ok := valueFromRequest(r, body, resolution.Profile.Mapping.EncryptedData)
		if !ok {
			http.Error(w, "encrypted_data not found by profile mapping", http.StatusBadRequest)
			return
		}

		env := &envelope{data: map[string]string{}}
		env.SetField("mapping", resolution.Profile.Mapping.ID, id)
		env.SetField("mapping", resolution.Profile.Mapping.EncryptedData, enc)
		if hint != "" && resolution.Profile.Mapping.ProfileID != "" {
			env.SetField("mapping", resolution.Profile.Mapping.ProfileID, hint)
		}

		outCanonical, err := runtime.HandleWithProfile(r.Context(), env, channelID, resolution.Profile)
		if err != nil {
			http.Error(w, "sync failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		out := SyncResponse{ID: outCanonical.ID, EncryptedData: outCanonical.EncryptedData}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	return &http.Server{Addr: addr, Handler: mux}
}

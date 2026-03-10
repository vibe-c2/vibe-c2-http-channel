package httpserver

import (
	"encoding/json"
	"net/http"

	coreErrors "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/errors"
	coreMatcher "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/matcher"
	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	coreRuntime "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/runtime"
	coreSync "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/syncclient"
)

type SyncRequest struct {
	ProfileID     string `json:"profile_id,omitempty"`
	ID            string `json:"id"`
	EncryptedData string `json:"encrypted_data"`
}

type SyncResponse struct {
	ID            string `json:"id"`
	EncryptedData string `json:"encrypted_data"`
}

type envelope struct {
	data map[string]string
}

func (e *envelope) SourceKey() string { return "http" }
func (e *envelope) GetField(location, key string) (string, bool) {
	v, ok := e.data[location+"."+key]
	return v, ok
}
func (e *envelope) SetField(location, key, value string) {
	e.data[location+"."+key] = value
}

func New(addr, channelID, c2SyncBaseURL string) *http.Server {
	runtime := coreRuntime.New(coreSync.NewHTTPClient(c2SyncBaseURL, nil))
	matcher := coreMatcher.New()
	profiles := []coreProfile.Profile{
		{
			ProfileID:       "default-http",
			ChannelType:     "http",
			Enabled:         true,
			DefaultFallback: true,
			Priority:        100,
			Mapping: coreProfile.Mapping{
				ProfileID:     "profile_id",
				ID:            "id",
				EncryptedData: "encrypted_data",
			},
		},
	}

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

		var in SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if in.ID == "" || in.EncryptedData == "" {
			http.Error(w, "id and encrypted_data are required", http.StatusBadRequest)
			return
		}

		env := &envelope{data: map[string]string{
			"mapping.id":             in.ID,
			"mapping.encrypted_data": in.EncryptedData,
		}}
		if in.ProfileID != "" {
			env.SetField("mapping", "profile_id", in.ProfileID)
		}

		resolution, err := matcher.Resolve(r.Context(), in.ProfileID, profiles)
		if err != nil {
			if code := coreErrors.Code(err); code == coreErrors.CodeProfileAmbiguous || code == coreErrors.CodeProfileNotFound {
				http.Error(w, "profile resolution failed: "+err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "profile resolution failed: "+err.Error(), http.StatusBadGateway)
			return
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

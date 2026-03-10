package httpserver

import (
	"encoding/json"
	"net/http"
)

type SyncRequest struct {
	ID            string `json:"id"`
	EncryptedData string `json:"encrypted_data"`
}

type SyncResponse struct {
	EncryptedData string `json:"encrypted_data"`
}

func New(addr string) *http.Server {
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

		// TODO: wire channel-core runtime + profile engine + C2 sync client.
		out := SyncResponse{EncryptedData: in.EncryptedData}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	return &http.Server{Addr: addr, Handler: mux}
}

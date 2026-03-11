package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	protocol "github.com/vibe-c2/vibe-c2-golang-protocol/protocol"
)

func newFakeC2(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/channel/sync", func(w http.ResponseWriter, r *http.Request) {
		var in protocol.InboundAgentMessage
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out := protocol.OutboundAgentMessage{
			MessageID:     "out-1",
			Type:          protocol.TypeOutboundAgentMessage,
			Version:       protocol.VersionV1,
			Timestamp:     "2026-03-11T00:00:00Z",
			Source:        protocol.SourceInfo{Module: "core", ModuleInstance: "main", Transport: "http", Tenant: "default"},
			ID:            in.ID + "-ack",
			EncryptedData: "resp:" + in.EncryptedData,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func TestSyncWithHintProfile(t *testing.T) {
	c2 := newFakeC2(t)
	defer c2.Close()

	profiles := []coreProfile.Profile{
		{ProfileID: "p1", ChannelType: "http", Enabled: true, Mapping: coreProfile.Mapping{ProfileID: "p", ID: "id_field", EncryptedData: "blob_field"}},
		{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 1, Mapping: coreProfile.Mapping{ID: "id", EncryptedData: "encrypted_data"}},
	}

	srv := New(":0", "http-main", c2.URL, profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	payload := []byte(`{"profile_id":"p1","id":"abc","encrypted_data":"xyz"}`)
	resp, err := http.Post(ts.URL+"/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	var out SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID != "abc-ack" || out.EncryptedData != "resp:xyz" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestSyncFallbackProfile(t *testing.T) {
	c2 := newFakeC2(t)
	defer c2.Close()

	profiles := []coreProfile.Profile{
		{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 5, Mapping: coreProfile.Mapping{ID: "id", EncryptedData: "encrypted_data"}},
	}

	srv := New(":0", "http-main", c2.URL, profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	payload := []byte(`{"id":"n1","encrypted_data":"blob"}`)
	resp, err := http.Post(ts.URL+"/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestSyncAmbiguousHint(t *testing.T) {
	c2 := newFakeC2(t)
	defer c2.Close()

	profiles := []coreProfile.Profile{
		{ProfileID: "a", ChannelType: "http", Enabled: true, Mapping: coreProfile.Mapping{ProfileID: "dup", ID: "id", EncryptedData: "encrypted_data"}},
		{ProfileID: "b", ChannelType: "http", Enabled: true, Mapping: coreProfile.Mapping{ProfileID: "dup", ID: "id", EncryptedData: "encrypted_data"}},
		{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 1, Mapping: coreProfile.Mapping{ID: "id", EncryptedData: "encrypted_data"}},
	}

	srv := New(":0", "http-main", c2.URL, profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	payload := []byte(`{"profile_id":"dup","id":"abc","encrypted_data":"xyz"}`)
	resp, err := http.Post(ts.URL+"/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

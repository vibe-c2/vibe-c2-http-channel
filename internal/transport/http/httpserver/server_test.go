package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
)

func TestObfuscationProfiles_Body(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()

	profiles := []coreProfile.Profile{
		{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 1, Mapping: coreProfile.Mapping{ID: "body:id", EncryptedData: "body:encrypted_data"}},
	}

	srv := New(":0", "http-main", c2.URL(), profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/sync", "application/json", bytes.NewReader([]byte(`{"id":"n1","encrypted_data":"blob"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	in := c2.LastInbound()
	if in.ID != "n1" || in.EncryptedData != "blob" {
		t.Fatalf("unexpected inbound to c2: %+v", in)
	}
}

func TestObfuscationProfiles_Header(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()
	profiles := []coreProfile.Profile{{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 1, Mapping: coreProfile.Mapping{ID: "header:X-Id", EncryptedData: "header:X-Payload"}}}

	srv := New(":0", "http-main", c2.URL(), profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/sync", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ID", "h-1")
	req.Header.Set("X-Payload", "hblob")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestObfuscationProfiles_Query(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()
	profiles := []coreProfile.Profile{{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 1, Mapping: coreProfile.Mapping{ID: "query:i", EncryptedData: "query:d"}}}

	srv := New(":0", "http-main", c2.URL(), profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/sync?i=q1&d=qblob", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestObfuscationProfiles_Cookie(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()
	profiles := []coreProfile.Profile{{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 1, Mapping: coreProfile.Mapping{ID: "cookie:cid", EncryptedData: "cookie:cpayload"}}}

	srv := New(":0", "http-main", c2.URL(), profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/sync", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "cid", Value: "c1"})
	req.AddCookie(&http.Cookie{Name: "cpayload", Value: "cblob"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestObfuscationProfiles_TransformBase64(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()
	profiles := []coreProfile.Profile{{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 1, Mapping: coreProfile.Mapping{ID: "body:id|base64", EncryptedData: "body:encrypted_data|base64"}}}

	srv := New(":0", "http-main", c2.URL(), profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	idIn := base64.StdEncoding.EncodeToString([]byte("agent-1"))
	blobIn := base64.StdEncoding.EncodeToString([]byte("cipher"))
	payload := []byte(`{"id":"` + idIn + `","encrypted_data":"` + blobIn + `"}`)
	resp, err := http.Post(ts.URL+"/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	in := c2.LastInbound()
	if in.ID != "agent-1" || in.EncryptedData != "cipher" {
		t.Fatalf("unexpected decoded inbound to c2: %+v", in)
	}

	var out SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	expectedID := base64.StdEncoding.EncodeToString([]byte("agent-1-ack"))
	expectedBlob := base64.StdEncoding.EncodeToString([]byte("resp:cipher"))
	if out.ID != expectedID || out.EncryptedData != expectedBlob {
		t.Fatalf("unexpected transformed outbound: %+v", out)
	}
}

func TestObfuscationProfiles_AmbiguousHint(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()

	profiles := []coreProfile.Profile{
		{ProfileID: "a", ChannelType: "http", Enabled: true, Mapping: coreProfile.Mapping{ProfileID: "dup", ID: "body:id", EncryptedData: "body:encrypted_data"}},
		{ProfileID: "b", ChannelType: "http", Enabled: true, Mapping: coreProfile.Mapping{ProfileID: "dup", ID: "body:id", EncryptedData: "body:encrypted_data"}},
		{ProfileID: "fallback", ChannelType: "http", Enabled: true, DefaultFallback: true, Priority: 1, Mapping: coreProfile.Mapping{ID: "body:id", EncryptedData: "body:encrypted_data"}},
	}

	srv := New(":0", "http-main", c2.URL(), profiles)
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

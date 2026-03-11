package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
)

func loadExampleProfiles(t *testing.T, file string) []coreProfile.Profile {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "examples", "profiles", file)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile example %s: %v", file, err)
	}
	profiles, err := coreProfile.ParseYAMLProfiles(b)
	if err != nil {
		t.Fatalf("parse profile example %s: %v", file, err)
	}
	return profiles
}

func TestObfuscationProfiles_Body(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()

	profiles := loadExampleProfiles(t, "body-default.yaml")

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
	profiles := loadExampleProfiles(t, "header-gateway.yaml")

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
	profiles := loadExampleProfiles(t, "query-beacon.yaml")

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
	profiles := loadExampleProfiles(t, "cookie-session.yaml")

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
	profiles := loadExampleProfiles(t, "body-base64.yaml")

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

func TestObfuscationProfiles_HintRouted(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()

	profiles := loadExampleProfiles(t, "hint-routed.yaml")
	srv := New(":0", "http-main", c2.URL(), profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	payload := []byte(`{"profile_id":"p1","id_field":"abc","blob_field":"xyz"}`)
	resp, err := http.Post(ts.URL+"/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	in := c2.LastInbound()
	if in.ID != "abc" || in.EncryptedData != "xyz" {
		t.Fatalf("unexpected routed inbound: %+v", in)
	}
}

func TestObfuscationProfiles_AmbiguousProfileSetRejected(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "examples", "profiles", "ambiguous-hint.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coreProfile.ParseYAMLProfiles(b); err == nil {
		t.Fatal("expected ambiguous profile set to be rejected")
	}
}

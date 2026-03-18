package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
)

func loadExampleProfiles(t *testing.T, files ...string) []coreProfile.Profile {
	t.Helper()
	all := make([]coreProfile.Profile, 0, len(files))
	for _, file := range files {
		path := filepath.Join("..", "..", "..", "..", "examples", "profiles", file)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read profile example %s: %v", file, err)
		}
		profiles, err := coreProfile.ParseYAMLProfiles(b)
		if err != nil {
			t.Fatalf("parse profile example %s: %v", file, err)
		}
		all = append(all, profiles...)
	}
	return all
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
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body: %s", resp.StatusCode, body)
	}

	in := c2.LastInbound()
	if in.ID != "n1" || in.EncryptedData != "blob" {
		t.Fatalf("unexpected inbound to c2: %+v", in)
	}

	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["encrypted_data"] != "resp:blob" {
		t.Fatalf("unexpected outbound encrypted_data: %s", out["encrypted_data"])
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
	req.Header.Set("X-Id", "h-1")
	req.Header.Set("X-Payload", "hblob")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body: %s", resp.StatusCode, body)
	}

	in := c2.LastInbound()
	if in.ID != "h-1" || in.EncryptedData != "hblob" {
		t.Fatalf("unexpected inbound to c2: %+v", in)
	}

	if got := resp.Header.Get("X-Payload"); got != "resp:hblob" {
		t.Fatalf("unexpected outbound X-Payload header: %s", got)
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
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body: %s", resp.StatusCode, body)
	}

	in := c2.LastInbound()
	if in.ID != "q1" || in.EncryptedData != "qblob" {
		t.Fatalf("unexpected inbound to c2: %+v", in)
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
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body: %s", resp.StatusCode, body)
	}

	in := c2.LastInbound()
	if in.ID != "c1" || in.EncryptedData != "cblob" {
		t.Fatalf("unexpected inbound to c2: %+v", in)
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
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body: %s", resp.StatusCode, body)
	}

	in := c2.LastInbound()
	if in.ID != "agent-1" || in.EncryptedData != "cipher" {
		t.Fatalf("unexpected decoded inbound to c2: %+v", in)
	}

	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	expectedBlob := base64.StdEncoding.EncodeToString([]byte("resp:cipher"))
	if out["encrypted_data"] != expectedBlob {
		t.Fatalf("unexpected transformed outbound encrypted_data: %s (want %s)", out["encrypted_data"], expectedBlob)
	}
}

func TestObfuscationProfiles_HintRouted(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()

	profiles := loadExampleProfiles(t, "hint-routed-p1.yaml", "hint-routed-fallback.yaml")
	srv := New(":0", "http-main", c2.URL(), profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	payload := []byte(`{"profile_id":"200","id_field":"abc","blob_field":"xyz"}`)
	resp, err := http.Post(ts.URL+"/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body: %s", resp.StatusCode, body)
	}
	in := c2.LastInbound()
	if in.ID != "abc" || in.EncryptedData != "xyz" {
		t.Fatalf("unexpected routed inbound: %+v", in)
	}
}

func TestDefaultProfile_CompositeBase64Body(t *testing.T) {
	c2 := newTestC2Core(t)
	defer c2.Close()

	profiles := loadExampleProfiles(t, "default.yaml")
	srv := New(":0", "http-main", c2.URL(), profiles)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	inRaw := base64.StdEncoding.EncodeToString([]byte("agent-7+ciphertext"))
	resp, err := http.Post(ts.URL+"/sync", "text/plain", bytes.NewReader([]byte(inRaw)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body: %s", resp.StatusCode, body)
	}

	in := c2.LastInbound()
	if in.ID != "agent-7" || in.EncryptedData != "ciphertext" {
		t.Fatalf("unexpected inbound to c2: %+v", in)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		t.Fatalf("response not base64: %v", err)
	}
	if string(decoded) != "resp:ciphertext" {
		t.Fatalf("unexpected decoded outbound body: %s", string(decoded))
	}
}

func TestObfuscationProfiles_AmbiguousProfileSetRejected(t *testing.T) {
	a := loadExampleProfiles(t, "ambiguous-a.yaml")
	b := loadExampleProfiles(t, "ambiguous-b.yaml")
	f := loadExampleProfiles(t, "ambiguous-fallback.yaml")
	combined := append(append(a, b...), f...)
	if err := coreProfile.ValidateSet(combined); err == nil {
		t.Fatal("expected ambiguous profile set to be rejected")
	}
}

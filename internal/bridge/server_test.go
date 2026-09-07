package bridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func TestAuthenticatedMailboxAndIdempotency(t *testing.T) {
	s, err := Start(t.TempDir(), map[string]string{"test": "tools"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	call := func(method, path, body, token, origin string) int {
		t.Helper()
		req, _ := http.NewRequest(method, s.Session.URL+path, bytes.NewBufferString(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	if call("GET", "/v1/tools", "", "", "") != 403 {
		t.Fatal("anonymous connection accepted")
	}
	if call("GET", "/v1/tools", "", s.Session.Token, "https://example.com") != 403 {
		t.Fatal("browser origin accepted")
	}
	if call("GET", "/v1/tools", "", s.Session.Token, "") != 200 {
		t.Fatal("authenticated tools unavailable")
	}
	body := `{"id":"edit1","type":"commands","ops":[{"op":"undo"}]}`
	if call("POST", "/v1/jobs", body, s.Session.Token, "") != 202 {
		t.Fatal("job not queued")
	}
	if call("POST", "/v1/jobs", body, s.Session.Token, "") != 200 {
		t.Fatal("retry not recognized")
	}
	req, ok := s.Next()
	if !ok || req.ID != "edit1" {
		t.Fatal("job missing")
	}
	if _, ok := s.Next(); ok {
		t.Fatal("retried edit was executed twice")
	}
	s.Complete(req.ID, map[string]bool{"ok": true}, nil)
	if call("POST", "/v1/jobs", `{"id":"edit1","type":"state"}`, s.Session.Token, "") != 409 {
		t.Fatal("id reused for a different operation")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil || session.PID != os.Getpid() {
		t.Fatal("bad discovery record")
	}
	s.Close()
	if _, err := os.Stat(s.path); !os.IsNotExist(err) {
		t.Fatal("discovery record survived disconnect")
	}
}

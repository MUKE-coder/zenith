package email

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Internal test: it reaches the unexported endpoint field to point the client
// at a local server rather than at Resend.
func fakeResend(t *testing.T, handler http.HandlerFunc) *Resend {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	r, err := NewResend("re_test_key")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	r.endpoint = srv.URL
	return r
}

func TestSend(t *testing.T) {
	var got struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
	}
	var auth string

	r := fakeResend(t, func(w http.ResponseWriter, req *http.Request) {
		auth = req.Header.Get("Authorization")
		body, _ := io.ReadAll(req.Body)
		json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"abc"}`))
	})

	err := r.Send(context.Background(), Message{
		From: "Zenith <a@example.com>", To: "owner@client.com",
		Subject: "June analytics", HTML: "<p>hi</p>",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if auth != "Bearer re_test_key" {
		t.Errorf("Authorization = %q", auth)
	}
	if got.From != "Zenith <a@example.com>" {
		t.Errorf("from = %q", got.From)
	}
	if len(got.To) != 1 || got.To[0] != "owner@client.com" {
		t.Errorf("to = %v", got.To)
	}
	if got.Subject != "June analytics" || got.HTML != "<p>hi</p>" {
		t.Errorf("subject/html = %q / %q", got.Subject, got.HTML)
	}
}

// Resend's own message is the useful part: "domain is not verified" tells the
// developer exactly what to fix. Replacing it with something vaguer would not.
func TestSendSurfacesResendsReason(t *testing.T) {
	r := fakeResend(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"name":"validation_error","message":"The example.com domain is not verified"}`))
	})

	err := r.Send(context.Background(), Message{
		From: "a@example.com", To: "b@example.com", Subject: "x", HTML: "<p></p>",
	})
	if err == nil {
		t.Fatal("a 403 was reported as success")
	}
	if !strings.Contains(err.Error(), "domain is not verified") {
		t.Errorf("error = %q, want Resend's reason", err)
	}
}

func TestSendOnUnparseableError(t *testing.T) {
	r := fakeResend(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<html>gateway error</html>`))
	})

	err := r.Send(context.Background(), Message{
		From: "a@example.com", To: "b@example.com", Subject: "x", HTML: "<p></p>",
	})
	if err == nil {
		t.Fatal("a 500 was reported as success")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want it to state the status", err)
	}
}

// A transport error must never carry the request back to a caller that logs
// it: the Authorization header travels with it.
func TestSendDoesNotLeakTheKeyOnTransportError(t *testing.T) {
	r, err := NewResend("re_live_super_secret")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	r.endpoint = "http://127.0.0.1:1" // nothing listens here

	err = r.Send(context.Background(), Message{
		From: "a@example.com", To: "b@example.com", Subject: "x", HTML: "<p></p>",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "re_live_super_secret") {
		t.Errorf("the error carries the API key: %v", err)
	}
}

func TestNewResendRequiresAKey(t *testing.T) {
	for _, key := range []string{"", "   "} {
		if _, err := NewResend(key); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("NewResend(%q) = %v, want ErrNotConfigured", key, err)
		}
	}
}

func TestSendRequiresFromAndTo(t *testing.T) {
	r := fakeResend(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("a message with no sender or recipient was sent")
	})

	for _, msg := range []Message{
		{To: "b@example.com"},
		{From: "a@example.com"},
	} {
		if err := r.Send(context.Background(), msg); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("Send(%+v) = %v, want ErrNotConfigured", msg, err)
		}
	}
}

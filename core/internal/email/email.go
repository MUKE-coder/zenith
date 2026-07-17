// Package email sends the monthly report.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message is one email.
type Message struct {
	From    string
	To      string
	Subject string
	HTML    string
}

// Sender delivers a message.
//
// An interface because the alternative is a test suite that either sends real
// email or does not test the code that sends email.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// ErrNotConfigured means no API key or from-address has been set.
var ErrNotConfigured = errors.New("email is not configured")

// resendEndpoint is Resend's send API.
const resendEndpoint = "https://api.resend.com/emails"

// Resend sends through the Resend API.
type Resend struct {
	apiKey string
	client *http.Client

	// endpoint is overridable so tests can point at a local server rather than
	// at Resend.
	endpoint string
}

// NewResend returns a Sender for an API key.
func NewResend(apiKey string) (*Resend, error) {
	return NewResendAt(apiKey, "")
}

// NewResendAt returns a Sender pointed at a specific endpoint.
//
// An empty endpoint means Resend itself. The override exists so a deployment
// can be verified against a mock without mailing real clients, which is
// otherwise the only way to find out whether email works.
func NewResendAt(apiKey, endpoint string) (*Resend, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrNotConfigured
	}
	if endpoint == "" {
		endpoint = resendEndpoint
	}
	return &Resend{
		apiKey:   apiKey,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 20 * time.Second},
	}, nil
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

type resendError struct {
	Message string `json:"message"`
	Name    string `json:"name"`
}

// Send delivers a message through Resend.
func (r *Resend) Send(ctx context.Context, msg Message) error {
	if msg.From == "" || msg.To == "" {
		return ErrNotConfigured
	}

	body, err := json.Marshal(resendRequest{
		From: msg.From, To: []string{msg.To}, Subject: msg.Subject, HTML: msg.HTML,
	})
	if err != nil {
		return fmt.Errorf("email: encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("email: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		// Never wrap: a transport error can carry the request, and the
		// Authorization header with it.
		return errors.New("email: could not reach Resend")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Resend's own message is the useful part -- "domain not verified", say --
	// and it is what the console shows the developer.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))

	var apiErr resendError
	if json.Unmarshal(raw, &apiErr) == nil && apiErr.Message != "" {
		return fmt.Errorf("resend: %s", apiErr.Message)
	}
	return fmt.Errorf("resend: returned %d", resp.StatusCode)
}

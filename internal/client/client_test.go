package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient wires a Client to a test server.
func newTestClient(t *testing.T, h http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New("test-key", "2026-04-15", WithBaseURL(srv.URL))
	return c, srv
}

func TestDoSendsAuthAndRevisionHeaders(t *testing.T) {
	var gotAuth, gotRevision, gotAccept, gotContentType string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotRevision = r.Header.Get("revision")
		gotAccept = r.Header.Get("Accept")
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))

	resp, err := c.Do(context.Background(), http.MethodPost, "/api/lists/", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if want := "Klaviyo-API-Key test-key"; gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
	if want := "2026-04-15"; gotRevision != want {
		t.Errorf("revision = %q, want %q", gotRevision, want)
	}
	if want := "application/vnd.api+json"; gotAccept != want {
		t.Errorf("Accept = %q, want %q", gotAccept, want)
	}
	if want := "application/vnd.api+json"; gotContentType != want {
		t.Errorf("Content-Type = %q, want %q", gotContentType, want)
	}
}

func TestDoOmitsContentTypeWhenNoBody(t *testing.T) {
	var gotContentType string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))

	resp, err := c.Do(context.Background(), http.MethodGet, "/api/lists/abc", nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if gotContentType != "" {
		t.Errorf("Content-Type should be unset on GET without body, got %q", gotContentType)
	}
}

func TestDoEncodesBodyAsJSON(t *testing.T) {
	var receivedBody string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))

	in := map[string]any{"name": "List A", "type": "list"}
	resp, err := c.Do(context.Background(), http.MethodPost, "/api/lists/", in)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(receivedBody), &parsed); err != nil {
		t.Fatalf("server received non-JSON body %q: %v", receivedBody, err)
	}
	if parsed["name"] != "List A" || parsed["type"] != "list" {
		t.Errorf("server received unexpected body: %v", parsed)
	}
}

func TestDoRetriesOn429UntilSuccess(t *testing.T) {
	var attempts atomic.Int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.Header().Set("Retry-After", "0") // don't actually sleep
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	resp, err := c.Do(context.Background(), http.MethodGet, "/api/lists/x", nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("final status = %d, want 200", resp.StatusCode)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestDoGivesUpAfterMaxRetries(t *testing.T) {
	var attempts atomic.Int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	resp, err := c.Do(context.Background(), http.MethodGet, "/api/lists/x", nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("final status = %d, want 429 (gave up)", resp.StatusCode)
	}
	// maxRetries=3 means 1 initial + 3 retries = 4 total attempts.
	if got := attempts.Load(); got != 4 {
		t.Errorf("expected 4 attempts (initial + 3 retries), got %d", got)
	}
}

func TestDoHonorsContextCancellation(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "10") // long enough that ctx cancel wins
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	resp, err := c.Do(ctx, http.MethodGet, "/api/lists/x", nil)
	if err == nil {
		if resp != nil {
			resp.Body.Close()
		}
		t.Fatalf("expected ctx error, got resp")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestDoReturnsNon429ResponseUntouched(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","code":"not_found","detail":"not here"}]}`)
	}))

	resp, err := c.Do(context.Background(), http.MethodGet, "/api/lists/missing", nil)
	if err != nil {
		t.Fatalf("Do should not error on 404 (caller decides): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDecodeErrorParsesJSONAPIShape(t *testing.T) {
	body := `{"errors":[{"id":"abc","status":"422","code":"invalid","title":"Invalid","detail":"name is required"}]}`
	resp := &http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	err := DecodeError(resp)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if apiErr.HTTPStatus != 422 {
		t.Errorf("HTTPStatus = %d, want 422", apiErr.HTTPStatus)
	}
	if len(apiErr.Details) != 1 || apiErr.Details[0].Detail != "name is required" {
		t.Errorf("Details = %+v", apiErr.Details)
	}
	if !strings.Contains(apiErr.Error(), "name is required") {
		t.Errorf("Error() = %q should mention the detail", apiErr.Error())
	}
}

func TestDecodeErrorFallsBackToRaw(t *testing.T) {
	body := `<html>500 Internal Server Error</html>`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	err := DecodeError(resp)
	apiErr := err.(*APIError)
	if apiErr.Raw != body {
		t.Errorf("Raw = %q, want %q", apiErr.Raw, body)
	}
	if !strings.Contains(apiErr.Error(), "500") {
		t.Errorf("Error() = %q should mention 500", apiErr.Error())
	}
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(&APIError{HTTPStatus: 404}) {
		t.Error("IsNotFound on 404 should be true")
	}
	if IsNotFound(&APIError{HTTPStatus: 500}) {
		t.Error("IsNotFound on 500 should be false")
	}
	if IsNotFound(nil) {
		t.Error("IsNotFound on nil should be false")
	}
}

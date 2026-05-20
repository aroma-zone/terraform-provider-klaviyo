package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// APIError is a typed error covering any non-2xx response from Klaviyo.
// Klaviyo follows JSON:API: errors come back as an array under
// `errors`, each with status/code/title/detail. We capture them all so
// callers can format diagnostics or check for specific codes.
type APIError struct {
	// HTTPStatus is the response status code.
	HTTPStatus int
	// Details holds every entry in the response's `errors` array. Empty
	// if the body wasn't parseable as a JSON:API error document.
	Details []APIErrorDetail
	// Raw is the unparsed response body, for diagnostics when the body
	// didn't follow the JSON:API shape.
	Raw string
}

// APIErrorDetail mirrors a single entry in Klaviyo's `errors` array.
// See https://developers.klaviyo.com/en/reference/api-overview#errors.
type APIErrorDetail struct {
	ID     string                 `json:"id,omitempty"`
	Status string                 `json:"status,omitempty"`
	Code   string                 `json:"code,omitempty"`
	Title  string                 `json:"title,omitempty"`
	Detail string                 `json:"detail,omitempty"`
	Source map[string]interface{} `json:"source,omitempty"`
}

func (e *APIError) Error() string {
	if len(e.Details) == 0 {
		// Fall back to raw body if we didn't get a structured error.
		if e.Raw != "" {
			return fmt.Sprintf("klaviyo: HTTP %d: %s", e.HTTPStatus, e.Raw)
		}
		return fmt.Sprintf("klaviyo: HTTP %d", e.HTTPStatus)
	}
	parts := make([]string, 0, len(e.Details))
	for _, d := range e.Details {
		switch {
		case d.Detail != "" && d.Title != "":
			parts = append(parts, fmt.Sprintf("%s: %s", d.Title, d.Detail))
		case d.Detail != "":
			parts = append(parts, d.Detail)
		case d.Title != "":
			parts = append(parts, d.Title)
		case d.Code != "":
			parts = append(parts, d.Code)
		}
	}
	return fmt.Sprintf("klaviyo: HTTP %d: %s", e.HTTPStatus, strings.Join(parts, "; "))
}

// IsNotFound reports whether err is an *APIError with a 404 status.
// Useful in Read implementations to remove the resource from state.
func IsNotFound(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.HTTPStatus == http.StatusNotFound
}

// DecodeError consumes the response body and returns a typed *APIError.
// The caller should call this only when resp.StatusCode is non-2xx; it
// is the caller's responsibility to close resp.Body afterwards (Do
// returns the response with the body still open).
func DecodeError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	apiErr := &APIError{
		HTTPStatus: resp.StatusCode,
		Raw:        strings.TrimSpace(string(body)),
	}
	// JSON:API error document: { "errors": [ { ... }, ... ] }
	var envelope struct {
		Errors []APIErrorDetail `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Errors) > 0 {
		apiErr.Details = envelope.Errors
		apiErr.Raw = "" // structured form is enough
	}
	return apiErr
}

package datasource_list

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"
)

// Test that the data source decodes a typical list response. We exercise
// the response-decoding directly (not via the full Plugin Framework
// machinery) — those wrappers are tested upstream.
func TestDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/lists/abc" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":{"type":"list","id":"abc","attributes":{
			"name":"Newsletter","opt_in_process":"single_opt_in",
			"created":"2026-05-20T10:00:00+00:00","updated":"2026-05-20T11:00:00+00:00"
		}}}`)
	}))
	defer srv.Close()

	c := client.New("k", "2026-04-15.pre", client.WithBaseURL(srv.URL))
	resp, err := c.Do(t.Context(), http.MethodGet, "/api/lists/abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.ID != "abc" || env.Data.Attributes.Name != "Newsletter" {
		t.Errorf("decoded wrong: %+v", env)
	}
	if env.Data.Attributes.OptInProcess == nil || *env.Data.Attributes.OptInProcess != "single_opt_in" {
		t.Errorf("opt_in_process: %v", env.Data.Attributes.OptInProcess)
	}
}

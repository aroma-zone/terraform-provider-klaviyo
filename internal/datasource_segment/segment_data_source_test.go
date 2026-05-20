package datasource_segment

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"
)

func TestDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":{"type":"segment","id":"ABC","attributes":{
			"name":"VIPs","definition":{"condition_groups":[]},
			"is_starred":true,"is_active":true,"is_processing":false
		}}}`)
	}))
	defer srv.Close()

	c := client.New("k", "2026-04-15.pre", client.WithBaseURL(srv.URL))
	resp, _ := c.Do(t.Context(), http.MethodGet, "/api/segments/ABC", nil)
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.ID != "ABC" || env.Data.Attributes.Name != "VIPs" {
		t.Errorf("decoded wrong: %+v", env)
	}
	if len(env.Data.Attributes.Definition) == 0 {
		t.Error("definition empty")
	}
}

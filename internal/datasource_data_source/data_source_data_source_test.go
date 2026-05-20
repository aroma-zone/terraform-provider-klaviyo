package datasource_data_source

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
		if r.URL.Path != "/api/data-sources/01ABC" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":{"type":"data-source","id":"01ABC","attributes":{
			"title":"Warehouse","visibility":"shared","description":"d","namespace":"custom-objects"
		}}}`)
	}))
	defer srv.Close()

	c := client.New("k", "2026-04-15.pre", client.WithBaseURL(srv.URL))
	resp, err := c.Do(t.Context(), http.MethodGet, "/api/data-sources/01ABC", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.ID != "01ABC" || env.Data.Attributes.Title != "Warehouse" {
		t.Errorf("decoded wrong: %+v", env)
	}
}

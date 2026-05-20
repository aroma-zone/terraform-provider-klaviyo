package datasource_custom_object

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
		_, _ = io.WriteString(w, `{"data":{"type":"object-schema","id":"01ABC","attributes":{
			"title":"Person","description":"a person","status":"DRAFT","visibility":"PRIVATE",
			"properties":[{"id":1,"name":"name","type":"STRING"}],
			"required":["name"]
		}}}`)
	}))
	defer srv.Close()

	c := client.New("k", "2026-04-15.pre", client.WithBaseURL(srv.URL))
	resp, _ := c.Do(t.Context(), http.MethodGet, "/api/object-schemas/01ABC", nil)
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.ID != "01ABC" || env.Data.Attributes.Title != "Person" {
		t.Errorf("decoded wrong: %+v", env)
	}
	if len(env.Data.Attributes.Properties) != 1 || env.Data.Attributes.Properties[0].Name != "name" {
		t.Errorf("properties: %+v", env.Data.Attributes.Properties)
	}
}

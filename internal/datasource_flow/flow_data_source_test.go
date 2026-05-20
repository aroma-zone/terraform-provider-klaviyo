package datasource_flow

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
		_, _ = io.WriteString(w, `{"data":{"type":"flow","id":"XVTP5Q","attributes":{
			"name":"Welcome","status":"live","archived":false,"trigger_type":"Added to List"
		}}}`)
	}))
	defer srv.Close()

	c := client.New("k", "2026-04-15.pre", client.WithBaseURL(srv.URL))
	resp, _ := c.Do(t.Context(), http.MethodGet, "/api/flows/XVTP5Q", nil)
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.ID != "XVTP5Q" || env.Data.Attributes.Status != "live" {
		t.Errorf("decoded wrong: %+v", env)
	}
}

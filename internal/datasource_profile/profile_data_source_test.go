package datasource_profile

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
		_, _ = io.WriteString(w, `{"data":{"type":"profile","id":"01XYZ","attributes":{
			"email":"a@b.com","first_name":"Sarah","properties":{"tier":"gold"}
		}}}`)
	}))
	defer srv.Close()

	c := client.New("k", "2026-04-15.pre", client.WithBaseURL(srv.URL))
	resp, _ := c.Do(t.Context(), http.MethodGet, "/api/profiles/01XYZ", nil)
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.ID != "01XYZ" || env.Data.Attributes.Email == nil || *env.Data.Attributes.Email != "a@b.com" {
		t.Errorf("decoded wrong: %+v", env)
	}
}

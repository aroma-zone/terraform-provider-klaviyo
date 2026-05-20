package datasource_webhook

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
		_, _ = io.WriteString(w, `{"data":{"type":"webhook","id":"01ABC","attributes":{
			"name":"X","endpoint_url":"https://e","enabled":true
		},"relationships":{"webhook-topics":{"data":[
			{"type":"webhook-topic","id":"event:klaviyo.sent_sms"}
		]}}}}`)
	}))
	defer srv.Close()

	c := client.New("k", "2026-04-15.pre", client.WithBaseURL(srv.URL))
	resp, _ := c.Do(t.Context(), http.MethodGet, "/api/webhooks/01ABC?include=webhook-topics", nil)
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.ID != "01ABC" || !env.Data.Attributes.Enabled {
		t.Errorf("decoded wrong: %+v", env)
	}
	if env.Data.Relationships == nil || len(env.Data.Relationships.WebhookTopics.Data) != 1 {
		t.Errorf("topics: %+v", env.Data.Relationships)
	}
}

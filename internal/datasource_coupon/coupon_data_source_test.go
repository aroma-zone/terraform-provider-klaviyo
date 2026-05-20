package datasource_coupon

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
		_, _ = io.WriteString(w, `{"data":{"type":"coupon","id":"10OFF","attributes":{
			"external_id":"10OFF","description":"10% off",
			"monitor_configuration":{"low_balance_threshold":500}
		}}}`)
	}))
	defer srv.Close()

	c := client.New("k", "2026-04-15.pre", client.WithBaseURL(srv.URL))
	resp, _ := c.Do(t.Context(), http.MethodGet, "/api/coupons/10OFF", nil)
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.ID != "10OFF" || env.Data.Attributes.ExternalID != "10OFF" {
		t.Errorf("decoded wrong: %+v", env)
	}
	if env.Data.Attributes.MonitorConfiguration == nil || *env.Data.Attributes.MonitorConfiguration.LowBalanceThreshold != 500 {
		t.Errorf("monitor_configuration: %+v", env.Data.Attributes.MonitorConfiguration)
	}
}

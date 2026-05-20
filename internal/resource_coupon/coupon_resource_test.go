package resource_coupon

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func newTestResource(t *testing.T, baseURL string) *couponResource {
	t.Helper()
	c := client.New("test-key", "2026-04-15", client.WithBaseURL(baseURL))
	return &couponResource{c: c}
}

func TestAttributesFromPlan_CreateIncludesExternalID(t *testing.T) {
	m := model{
		ExternalID:  types.StringValue("10OFF"),
		Description: types.StringValue("ten percent"),
	}
	a := attributesFromPlan(m, true)
	if a.ExternalID != "10OFF" {
		t.Errorf("external_id = %q", a.ExternalID)
	}
	if a.Description == nil || *a.Description != "ten percent" {
		t.Errorf("description wrong: %v", a.Description)
	}
}

func TestAttributesFromPlan_UpdateOmitsExternalID(t *testing.T) {
	m := model{
		ExternalID:  types.StringValue("10OFF"),
		Description: types.StringValue("new desc"),
	}
	a := attributesFromPlan(m, false)
	if a.ExternalID != "" {
		t.Errorf("update should not include external_id, got %q", a.ExternalID)
	}
	// omitempty + "" means JSON also omits it.
	b, _ := json.Marshal(a)
	if strings.Contains(string(b), "external_id") {
		t.Errorf("JSON should not contain external_id on update: %s", b)
	}
}

func TestAttributesFromPlan_MonitorConfigSerializes(t *testing.T) {
	m := model{
		ExternalID: types.StringValue("X"),
		MonitorConfiguration: &monitorConfigBlock{
			LowBalanceThreshold: types.Int64Value(500),
		},
	}
	a := attributesFromPlan(m, true)
	if a.MonitorConfiguration == nil || a.MonitorConfiguration.LowBalanceThreshold == nil || *a.MonitorConfiguration.LowBalanceThreshold != 500 {
		t.Errorf("monitor_configuration wrong: %+v", a.MonitorConfiguration)
	}
	b, _ := json.Marshal(a)
	if !strings.Contains(string(b), `"low_balance_threshold":500`) {
		t.Errorf("JSON missing threshold: %s", b)
	}
}

func TestAttributesFromPlan_OmitsNullMonitorConfig(t *testing.T) {
	m := model{
		ExternalID:           types.StringValue("X"),
		MonitorConfiguration: nil,
	}
	a := attributesFromPlan(m, true)
	if a.MonitorConfiguration != nil {
		t.Error("nil block should produce nil DTO")
	}
}

func TestMergeIntoModel_FullPayload(t *testing.T) {
	desc := "ten percent"
	thr := int64(500)
	r := resourceObject{
		Type: "coupon",
		ID:   "10OFF",
		Attributes: attributes{
			ExternalID:  "10OFF",
			Description: &desc,
			MonitorConfiguration: &monitorConfiguration{
				LowBalanceThreshold: &thr,
			},
		},
	}
	var m model
	mergeIntoModel(&m, r)
	if m.ID.ValueString() != "10OFF" {
		t.Errorf("id = %v", m.ID)
	}
	if m.ExternalID.ValueString() != "10OFF" {
		t.Errorf("external_id = %v", m.ExternalID)
	}
	if m.Description.ValueString() != "ten percent" {
		t.Errorf("description = %v", m.Description)
	}
	if m.MonitorConfiguration == nil || m.MonitorConfiguration.LowBalanceThreshold.ValueInt64() != 500 {
		t.Errorf("monitor = %+v", m.MonitorConfiguration)
	}
}

func TestMergeIntoModel_NullMonitorConfig(t *testing.T) {
	r := resourceObject{
		ID: "X",
		Attributes: attributes{
			ExternalID: "X",
		},
	}
	var m model
	mergeIntoModel(&m, r)
	if m.MonitorConfiguration != nil {
		t.Errorf("expected nil monitor block, got %+v", m.MonitorConfiguration)
	}
}

func TestSend_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{
			"data":{"type":"coupon","id":"10OFF","attributes":{
				"external_id":"10OFF","description":"ten percent","monitor_configuration":null
			}}
		}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	body := envelope{Data: resourceObject{Type: apiType, Attributes: attributes{ExternalID: "10OFF"}}}
	out, err := r.send(t.Context(), http.MethodPost, "/api/coupons/", body, http.StatusCreated)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.ID != "10OFF" || out.Attributes.ExternalID != "10OFF" {
		t.Errorf("decoded wrong: %+v", out)
	}
}

func TestSend_404Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","detail":"no coupon"}]}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	_, err := r.send(t.Context(), http.MethodGet, "/api/coupons/missing", nil, http.StatusOK)
	if err == nil {
		t.Fatal("expected err")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

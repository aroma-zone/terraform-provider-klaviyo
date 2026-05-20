package resource_segment

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func newTestResource(t *testing.T, baseURL string) *segmentResource {
	t.Helper()
	c := client.New("test-key", "2026-04-15.pre", client.WithBaseURL(baseURL))
	return &segmentResource{c: c}
}

func TestAttributesFromPlan_RequiredAndOptional(t *testing.T) {
	plan := model{
		Name:       types.StringValue("VIPs"),
		Definition: jsontypes.NewNormalizedValue(`{"condition_groups":[]}`),
		IsStarred:  types.BoolValue(true),
		IsActive:   types.BoolValue(false),
	}
	a := attributesFromPlan(plan)
	if a.Name != "VIPs" {
		t.Errorf("name = %q", a.Name)
	}
	if string(a.Definition) != `{"condition_groups":[]}` {
		t.Errorf("definition = %s", a.Definition)
	}
	if a.IsStarred == nil || !*a.IsStarred {
		t.Errorf("is_starred wrong: %v", a.IsStarred)
	}
	if a.IsActive == nil || *a.IsActive {
		t.Errorf("is_active wrong: %v", a.IsActive)
	}
}

func TestMergeIntoModel(t *testing.T) {
	starred := true
	active := true
	processing := false
	created := "2026-05-20T10:00:00+00:00"
	r := resourceObject{
		ID: "ABC",
		Attributes: attributes{
			Name:         "VIPs",
			Definition:   json.RawMessage(`{"condition_groups":[]}`),
			IsStarred:    &starred,
			IsActive:     &active,
			IsProcessing: &processing,
			Created:      &created,
		},
	}
	var m model
	mergeIntoModel(&m, r)
	if m.ID.ValueString() != "ABC" || m.Name.ValueString() != "VIPs" {
		t.Errorf("id/name: %v %v", m.ID, m.Name)
	}
	if !m.IsStarred.ValueBool() || !m.IsActive.ValueBool() {
		t.Errorf("starred/active: %v %v", m.IsStarred, m.IsActive)
	}
	if m.IsProcessing.ValueBool() {
		t.Errorf("is_processing should be false")
	}
}

func TestSend_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{
			"data":{"type":"segment","id":"ABC","attributes":{
				"name":"VIPs","definition":{"condition_groups":[]},
				"is_starred":true,"is_active":true,"is_processing":false
			}}
		}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	body := envelope{Data: resourceObject{
		Type:       apiType,
		Attributes: attributes{Name: "VIPs", Definition: json.RawMessage(`{}`)},
	}}
	out, err := r.send(t.Context(), http.MethodPost, "/api/segments/", body, http.StatusCreated)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.ID != "ABC" {
		t.Errorf("decoded wrong: %+v", out)
	}
}

func TestSend_404Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","detail":"missing"}]}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	_, err := r.send(t.Context(), http.MethodGet, "/api/segments/missing", nil, http.StatusOK)
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

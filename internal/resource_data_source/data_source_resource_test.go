package resource_data_source

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

func newTestResource(t *testing.T, baseURL string) *dataSourceResource {
	t.Helper()
	c := client.New("test-key", "2026-04-15", client.WithBaseURL(baseURL))
	return &dataSourceResource{c: c}
}

func TestAttributesFromPlan_OnlyRequired(t *testing.T) {
	m := model{
		Title:       types.StringValue("My Source"),
		Visibility:  types.StringUnknown(),
		Description: types.StringNull(),
		Namespace:   types.StringUnknown(),
	}
	a := attributesFromPlan(m)
	if a.Title != "My Source" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Visibility != nil || a.Description != nil || a.Namespace != nil {
		t.Errorf("unknown/null optional fields should be nil: %+v", a)
	}
}

func TestAttributesFromPlan_AllSet(t *testing.T) {
	m := model{
		Title:       types.StringValue("My Source"),
		Visibility:  types.StringValue("shared"),
		Description: types.StringValue("desc"),
		Namespace:   types.StringValue("custom-objects"),
	}
	a := attributesFromPlan(m)
	if a.Visibility == nil || *a.Visibility != "shared" {
		t.Errorf("Visibility wrong: %v", a.Visibility)
	}
	if a.Description == nil || *a.Description != "desc" {
		t.Errorf("Description wrong: %v", a.Description)
	}
	if a.Namespace == nil || *a.Namespace != "custom-objects" {
		t.Errorf("Namespace wrong: %v", a.Namespace)
	}
}

func TestEnvelopeMarshalsCorrectly(t *testing.T) {
	body := envelope{Data: resourceObject{
		Type:       apiType,
		Attributes: attributesFromPlan(model{Title: types.StringValue("X"), Visibility: types.StringValue("private")}),
	}}
	b, _ := json.Marshal(body)
	var got map[string]map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["data"]["type"] != "data-source" {
		t.Errorf("type = %v, want data-source", got["data"]["type"])
	}
	attrs := got["data"]["attributes"].(map[string]any)
	if attrs["title"] != "X" || attrs["visibility"] != "private" {
		t.Errorf("attrs = %v", attrs)
	}
	if _, present := got["data"]["id"]; present {
		t.Error("create payload must not include id")
	}
}

func TestMergeIntoModel_PopulatesAll(t *testing.T) {
	vis := "shared"
	desc := "a description"
	ns := "custom-objects"
	r := resourceObject{
		ID: "01ABC",
		Attributes: attributes{
			Title: "My Source", Visibility: &vis, Description: &desc, Namespace: &ns,
		},
	}
	var m model
	mergeIntoModel(&m, r)
	if m.ID.ValueString() != "01ABC" ||
		m.Title.ValueString() != "My Source" ||
		m.Visibility.ValueString() != "shared" ||
		m.Description.ValueString() != "a description" ||
		m.Namespace.ValueString() != "custom-objects" {
		t.Errorf("model = %+v", m)
	}
}

func TestSend_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{
			"data":{"type":"data-source","id":"01ABC","attributes":{
				"title":"X","visibility":"private","description":"","namespace":"custom-objects"
			}}
		}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	body := envelope{Data: resourceObject{Type: apiType, Attributes: attributes{Title: "X"}}}
	out, err := r.send(t.Context(), http.MethodPost, "/api/data-sources/", body, http.StatusCreated)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.ID != "01ABC" || out.Attributes.Title != "X" {
		t.Errorf("decoded wrong: %+v", out)
	}
}

func TestSend_404PropagatesAsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","detail":"gone"}]}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	_, err := r.send(t.Context(), http.MethodGet, "/api/data-sources/missing", nil, http.StatusOK)
	if err == nil {
		t.Fatal("expected err")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound to be true, got %v", err)
	}
	if !strings.Contains(err.Error(), "gone") {
		t.Errorf("error should surface server detail: %v", err)
	}
}

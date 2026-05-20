package resource_custom_object

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

func newTestResource(t *testing.T, baseURL string) *customObjectResource {
	t.Helper()
	c := client.New("test-key", "2026-04-15.pre", client.WithBaseURL(baseURL))
	return &customObjectResource{c: c}
}

func TestAttributesFromPlan_MinimalRequiredFields(t *testing.T) {
	m := model{
		Title: types.StringValue("Person"),
		Properties: []propertyBlock{
			{ID: types.Int64Value(1), Name: types.StringValue("name"), Type: types.StringValue("STRING")},
		},
	}
	a := attributesFromPlan(m)
	if a.Title != "Person" {
		t.Errorf("title = %q", a.Title)
	}
	if len(a.Properties) != 1 || a.Properties[0].Name != "name" || a.Properties[0].Type != "STRING" {
		t.Errorf("properties wrong: %+v", a.Properties)
	}
	if a.Description != nil || a.Status != nil || a.Required != nil {
		t.Errorf("optional fields should be nil: %+v", a)
	}
}

func TestAttributesFromPlan_AllFieldsPopulated(t *testing.T) {
	desc := "person description"
	m := model{
		Title:       types.StringValue("Person"),
		Description: types.StringValue("a custom object"),
		Status:      types.StringValue("ACTIVE"),
		Required:    []types.String{types.StringValue("name"), types.StringValue("age")},
		Properties: []propertyBlock{
			{ID: types.Int64Value(1), Name: types.StringValue("name"), Type: types.StringValue("STRING"), Description: types.StringValue(desc)},
			{ID: types.Int64Value(2), Name: types.StringValue("age"), Type: types.StringValue("INT")},
		},
	}
	a := attributesFromPlan(m)
	if *a.Status != "ACTIVE" {
		t.Errorf("status = %v", a.Status)
	}
	if a.Required == nil || len(*a.Required) != 2 || (*a.Required)[1] != "age" {
		t.Errorf("required wrong: %+v", a.Required)
	}
	if a.Properties[0].Description == nil || *a.Properties[0].Description != desc {
		t.Errorf("first property description: %v", a.Properties[0].Description)
	}
	if a.Properties[1].Description != nil {
		t.Errorf("second property description should be nil")
	}
}

func TestAttributesFromPlan_EmptyPropertiesProducesEmptyJSONArray(t *testing.T) {
	// Klaviyo requires properties; we still want json marshaling to
	// produce `"properties":[]` (not `"properties":null`) so the API
	// can return a clear validation error rather than 500ing.
	m := model{Title: types.StringValue("Empty")}
	a := attributesFromPlan(m)
	b, _ := json.Marshal(a)
	if !strings.Contains(string(b), `"properties":[]`) {
		t.Errorf("expected empty array for properties, got: %s", b)
	}
}

func TestMergeIntoModel_PopulatesAllFields(t *testing.T) {
	desc := "person description"
	status := "ACTIVE"
	visibility := "PRIVATE"
	pubAt := "2026-05-20T10:00:00+00:00"
	required := []string{"name"}
	propDesc := "first prop"
	r := resourceObject{
		ID: "01ABC",
		Attributes: attributes{
			Title:       "Person",
			Description: &desc,
			Status:      &status,
			Visibility:  &visibility,
			PublishedAt: &pubAt,
			Required:    &required,
			Properties: []apiProperty{
				{ID: 1, Name: "name", Type: "STRING", Description: &propDesc},
				{ID: 2, Name: "age", Type: "INT"},
			},
		},
	}
	var m model
	mergeIntoModel(&m, r)
	if m.ID.ValueString() != "01ABC" || m.Title.ValueString() != "Person" {
		t.Errorf("id/title: %v %v", m.ID, m.Title)
	}
	if m.Status.ValueString() != "ACTIVE" || m.Visibility.ValueString() != "PRIVATE" {
		t.Errorf("status/visibility: %v %v", m.Status, m.Visibility)
	}
	if len(m.Required) != 1 || m.Required[0].ValueString() != "name" {
		t.Errorf("required: %+v", m.Required)
	}
	if len(m.Properties) != 2 || m.Properties[0].Description.ValueString() != "first prop" || !m.Properties[1].Description.IsNull() {
		t.Errorf("properties: %+v", m.Properties)
	}
}

func TestSend_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{
			"data":{"type":"object-schema","id":"01ABC","attributes":{
				"title":"Person","description":"d","status":"DRAFT","visibility":"PRIVATE",
				"properties":[{"id":1,"name":"name","type":"STRING"}],
				"required":["name"]
			}}
		}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	out, err := r.send(t.Context(), http.MethodPost, "/api/object-schemas/",
		envelope{Data: resourceObject{Type: apiType, Attributes: attributes{Title: "Person", Properties: []apiProperty{{ID: 1, Name: "name", Type: "STRING"}}}}},
		http.StatusCreated)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.ID != "01ABC" || out.Attributes.Title != "Person" || len(out.Attributes.Properties) != 1 {
		t.Errorf("decoded wrong: %+v", out)
	}
}

func TestSend_404Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","detail":"not found"}]}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	_, err := r.send(t.Context(), http.MethodGet, "/api/object-schemas/missing", nil, http.StatusOK)
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

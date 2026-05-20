package resource_profile

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

func newTestResource(t *testing.T, baseURL string) *profileResource {
	t.Helper()
	c := client.New("test-key", "2026-04-15.pre", client.WithBaseURL(baseURL))
	return &profileResource{c: c}
}

func TestAttributesFromPlan_OnlySetFields(t *testing.T) {
	plan := model{
		Email:     types.StringValue("a@b.com"),
		FirstName: types.StringValue("Sarah"),
		// Other fields left unknown/null
	}
	a := attributesFromPlan(plan)
	if a.Email == nil || *a.Email != "a@b.com" {
		t.Errorf("email = %v", a.Email)
	}
	if a.FirstName == nil || *a.FirstName != "Sarah" {
		t.Errorf("first_name = %v", a.FirstName)
	}
	if a.PhoneNumber != nil || a.LastName != nil || a.ExternalID != nil {
		t.Errorf("unset fields should be nil: %+v", a)
	}
}

func TestAttributesFromPlan_PropertiesPassThroughAsJSON(t *testing.T) {
	plan := model{
		Email:      types.StringValue("a@b.com"),
		Properties: jsontypes.NewNormalizedValue(`{"tier":"gold"}`),
	}
	a := attributesFromPlan(plan)
	if string(a.Properties) != `{"tier":"gold"}` {
		t.Errorf("properties = %s", a.Properties)
	}
}

func TestMergeIntoModel(t *testing.T) {
	email := "a@b.com"
	first := "Sarah"
	created := "2026-05-20T10:00:00+00:00"
	r := resourceObject{
		ID: "01ABC",
		Attributes: attributes{
			Email:      &email,
			FirstName:  &first,
			Created:    &created,
			Properties: json.RawMessage(`{"tier":"gold"}`),
		},
	}
	var m model
	mergeIntoModel(&m, r)
	if m.ID.ValueString() != "01ABC" || m.Email.ValueString() != "a@b.com" {
		t.Errorf("id/email: %v %v", m.ID, m.Email)
	}
	if m.FirstName.ValueString() != "Sarah" {
		t.Errorf("first_name: %v", m.FirstName)
	}
	// Unset fields must become Null, not leak from the (zero-value) starting state.
	if !m.LastName.IsNull() {
		t.Errorf("last_name should be null, got %v", m.LastName)
	}
	if m.Properties.ValueString() != `{"tier":"gold"}` {
		t.Errorf("properties: %v", m.Properties)
	}
}

func TestSend_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{
			"data":{"type":"profile","id":"01ABC","attributes":{
				"email":"a@b.com","first_name":"Sarah"
			}}
		}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	em := "a@b.com"
	body := envelope{Data: resourceObject{Type: apiType, Attributes: attributes{Email: &em}}}
	out, err := r.send(t.Context(), http.MethodPost, "/api/profiles/", body, http.StatusCreated)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.ID != "01ABC" || out.Attributes.Email == nil {
		t.Errorf("decoded wrong: %+v", out)
	}
}

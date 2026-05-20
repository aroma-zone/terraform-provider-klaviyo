package resource_flow

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func newTestResource(t *testing.T, baseURL string) *flowResource {
	t.Helper()
	c := client.New("test-key", "2026-04-15.pre", client.WithBaseURL(baseURL))
	return &flowResource{c: c}
}

func TestCreatePayloadShape(t *testing.T) {
	// Verifies that the JSON sent on Create has the expected
	// data.attributes.{name,definition} shape, with definition
	// embedded as a JSON object (not a string-of-JSON).
	defJSON := `{"triggers":[{"type":"list"}]}`
	plan := model{
		Name:       types.StringValue("Welcome"),
		Definition: jsontypes.NewNormalizedValue(defJSON),
	}
	body := envelope{Data: resourceObject{
		Type: apiType,
		Attributes: attributes{
			Name:       plan.Name.ValueString(),
			Definition: json.RawMessage(plan.Definition.ValueString()),
		},
	}}
	b, _ := json.Marshal(body)

	var got map[string]map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	data := got["data"]
	if data["type"] != "flow" {
		t.Errorf("type = %v", data["type"])
	}
	attrs := data["attributes"].(map[string]any)
	if attrs["name"] != "Welcome" {
		t.Errorf("name = %v", attrs["name"])
	}
	def := attrs["definition"].(map[string]any)
	if def["triggers"] == nil {
		t.Errorf("definition should be parsed as object: %v", def)
	}
}

func TestMergeIntoModel_LeavesDefinitionAlone(t *testing.T) {
	// Klaviyo's read response does NOT include the definition; the
	// model field should remain whatever the caller already had.
	archived := false
	trig := "Added to List"
	created := "2026-05-20T10:00:00+00:00"
	r := resourceObject{
		ID: "XVTP5Q",
		Attributes: attributes{
			Name:        "Welcome",
			Status:      "draft",
			Archived:    &archived,
			TriggerType: &trig,
			Created:     &created,
		},
	}
	m := model{
		Definition: jsontypes.NewNormalizedValue(`{"keep":"this"}`),
	}
	mergeIntoModel(&m, r)
	if m.ID.ValueString() != "XVTP5Q" || m.Name.ValueString() != "Welcome" {
		t.Errorf("id/name: %v %v", m.ID, m.Name)
	}
	if m.Status.ValueString() != "draft" {
		t.Errorf("status: %v", m.Status)
	}
	if m.Archived.ValueBool() {
		t.Errorf("archived should be false")
	}
	if m.TriggerType.ValueString() != "Added to List" {
		t.Errorf("trigger_type: %v", m.TriggerType)
	}
	// Definition must remain untouched.
	if m.Definition.ValueString() != `{"keep":"this"}` {
		t.Errorf("definition was touched: %v", m.Definition)
	}
}

func TestSend_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{
			"data":{"type":"flow","id":"XVTP5Q","attributes":{
				"name":"Welcome","status":"draft","archived":false,"trigger_type":"Added to List"
			}}
		}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	body := envelope{Data: resourceObject{
		Type: apiType,
		Attributes: attributes{
			Name:       "Welcome",
			Definition: json.RawMessage(`{"triggers":[]}`),
		},
	}}
	out, err := r.send(t.Context(), http.MethodPost, "/api/flows/", body, http.StatusCreated)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.ID != "XVTP5Q" || out.Attributes.Status != "draft" {
		t.Errorf("decoded wrong: %+v", out)
	}
}

func TestSend_404Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","detail":"no flow"}]}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	_, err := r.send(t.Context(), http.MethodGet, "/api/flows/missing", nil, http.StatusOK)
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestUpdatePayloadOnlySendsStatus(t *testing.T) {
	// The Update path must POST only the status field on attributes,
	// not name or definition.
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":{"type":"flow","id":"X","attributes":{"status":"live"}}}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	body := envelope{Data: resourceObject{
		Type:       apiType,
		ID:         "X",
		Attributes: attributes{Status: "live"},
	}}
	if _, err := r.send(t.Context(), http.MethodPatch, "/api/flows/X", body, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(receivedBody, `"name":`) || strings.Contains(receivedBody, `"definition":`) {
		t.Errorf("update payload should be status-only, got: %s", receivedBody)
	}
	if !strings.Contains(receivedBody, `"status":"live"`) {
		t.Errorf("update payload missing status: %s", receivedBody)
	}
}

package resource_list

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

// newTestResource builds a listResource wired to a *client.Client
// pointed at the given test server URL.
func newTestResource(t *testing.T, baseURL string) *listResource {
	t.Helper()
	c := client.New("test-key", "2026-04-15", client.WithBaseURL(baseURL))
	return &listResource{c: c}
}

// These tests target the pure logic — attributesFromPlan and
// mergeIntoModel — plus a round-trip against an httptest server using
// the package's own client wiring. We intentionally don't spin up a
// full Terraform plugin server here: CRUD plumbing lives in
// terraform-plugin-framework's well-tested code, and the parts unique
// to this provider are the JSON:API translation and the HTTP shape.

func TestAttributesFromPlan_OmitsUnknownOptIn(t *testing.T) {
	m := model{
		Name:         types.StringValue("My List"),
		OptInProcess: types.StringUnknown(),
	}
	a := attributesFromPlan(m)
	if a.Name != "My List" {
		t.Errorf("Name = %q, want %q", a.Name, "My List")
	}
	if a.OptInProcess != nil {
		t.Errorf("OptInProcess should be nil when unknown, got %v", *a.OptInProcess)
	}
}

func TestAttributesFromPlan_OmitsNullOptIn(t *testing.T) {
	m := model{
		Name:         types.StringValue("My List"),
		OptInProcess: types.StringNull(),
	}
	a := attributesFromPlan(m)
	if a.OptInProcess != nil {
		t.Errorf("OptInProcess should be nil when null, got %v", *a.OptInProcess)
	}
}

func TestAttributesFromPlan_SendsValue(t *testing.T) {
	m := model{
		Name:         types.StringValue("My List"),
		OptInProcess: types.StringValue("double_opt_in"),
	}
	a := attributesFromPlan(m)
	if a.OptInProcess == nil || *a.OptInProcess != "double_opt_in" {
		t.Errorf("OptInProcess wrong: %v", a.OptInProcess)
	}
}

func TestAttributesFromPlan_MarshalsJSONEnvelope(t *testing.T) {
	body := envelope{Data: resourceObject{
		Type: "list",
		Attributes: attributesFromPlan(model{
			Name:         types.StringValue("Newsletter"),
			OptInProcess: types.StringValue("single_opt_in"),
		}),
	}}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	data := got["data"]
	if data["type"] != "list" {
		t.Errorf("type = %v, want list", data["type"])
	}
	attrs := data["attributes"].(map[string]any)
	if attrs["name"] != "Newsletter" || attrs["opt_in_process"] != "single_opt_in" {
		t.Errorf("attrs = %v", attrs)
	}
	// id should NOT be present on create payloads (omitempty).
	if _, ok := data["id"]; ok {
		t.Errorf("create payload must not include id, got %v", data)
	}
}

func TestMergeIntoModel_PopulatesAllFields(t *testing.T) {
	opt := "double_opt_in"
	r := resourceObject{
		Type: "list",
		ID:   "Y6nRLr",
		Attributes: attributes{
			Name:         "Newsletter",
			OptInProcess: &opt,
			Created:      "2026-05-20T10:00:00+00:00",
			Updated:      "2026-05-20T11:00:00+00:00",
		},
	}
	var m model
	mergeIntoModel(&m, r)
	if m.ID.ValueString() != "Y6nRLr" ||
		m.Name.ValueString() != "Newsletter" ||
		m.OptInProcess.ValueString() != "double_opt_in" ||
		m.Created.ValueString() != "2026-05-20T10:00:00+00:00" ||
		m.Updated.ValueString() != "2026-05-20T11:00:00+00:00" {
		t.Errorf("model = %+v", m)
	}
}

func TestMergeIntoModel_NullOptInWhenServerNulls(t *testing.T) {
	r := resourceObject{
		ID:         "abc",
		Attributes: attributes{Name: "x"}, // OptInProcess intentionally nil
	}
	var m model
	mergeIntoModel(&m, r)
	if !m.OptInProcess.IsNull() {
		t.Errorf("OptInProcess should be Null, got %v", m.OptInProcess)
	}
}

// TestSend_HappyPathDecodesEnvelope spins up an httptest server, builds
// a real *client.Client + listResource, and confirms a successful POST
// is decoded.
func TestSend_HappyPathDecodesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/lists/" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"name":"Newsletter"`) {
			t.Errorf("server didn't see name: %s", b)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{
			"data":{
				"type":"list",
				"id":"NEW-ID",
				"attributes":{
					"name":"Newsletter",
					"created":"2026-05-20T10:00:00+00:00",
					"updated":"2026-05-20T10:00:00+00:00"
				}
			}
		}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	body := envelope{Data: resourceObject{
		Type:       "list",
		Attributes: attributes{Name: "Newsletter"},
	}}
	out, err := r.send(t.Context(), http.MethodPost, "/api/lists/", body, http.StatusCreated)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.ID != "NEW-ID" || out.Attributes.Name != "Newsletter" {
		t.Errorf("decoded wrong: %+v", out)
	}
}

func TestSend_DecodesErrorOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","detail":"List not found"}]}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	_, err := r.send(t.Context(), http.MethodGet, "/api/lists/missing", nil, http.StatusOK)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "List not found") {
		t.Errorf("error should surface the API message, got: %v", err)
	}
}

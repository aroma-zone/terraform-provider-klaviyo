package resource_webhook

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

func newTestResource(t *testing.T, baseURL string) *webhookResource {
	t.Helper()
	c := client.New("test-key", "2026-04-15.pre", client.WithBaseURL(baseURL))
	return &webhookResource{c: c}
}

func TestAttributesFromPlan_CreatePath(t *testing.T) {
	plan := model{
		Name:        types.StringValue("SMS Events"),
		EndpointURL: types.StringValue("https://example.com/hook"),
		SecretKey:   types.StringValue("supersecret"),
	}
	a := attributesFromPlan(plan, true)
	if a.Name != "SMS Events" || a.EndpointURL != "https://example.com/hook" || a.SecretKey != "supersecret" {
		t.Errorf("attrs: %+v", a)
	}
}

func TestRelationshipsFromPlan(t *testing.T) {
	plan := model{
		WebhookTopics: []types.String{
			types.StringValue("event:klaviyo.sent_sms"),
			types.StringValue("event:klaviyo.failed_sms"),
		},
	}
	rels := relationshipsFromPlan(plan)
	if rels == nil || len(rels.WebhookTopics.Data) != 2 {
		t.Fatalf("relationships: %+v", rels)
	}
	if rels.WebhookTopics.Data[0].Type != "webhook-topic" {
		t.Errorf("topic type = %q", rels.WebhookTopics.Data[0].Type)
	}
}

func TestCreatePayloadShape(t *testing.T) {
	plan := model{
		Name:        types.StringValue("X"),
		EndpointURL: types.StringValue("https://e/u"),
		SecretKey:   types.StringValue("s"),
		WebhookTopics: []types.String{
			types.StringValue("event:klaviyo.sent_sms"),
		},
	}
	body := writeEnvelope{Data: writeResourceObject{
		Type:          apiType,
		Attributes:    attributesFromPlan(plan, true),
		Relationships: relationshipsFromPlan(plan),
	}}
	b, _ := json.Marshal(body)
	got := string(b)
	if !strings.Contains(got, `"webhook-topics":`) {
		t.Errorf("missing relationships: %s", got)
	}
	if !strings.Contains(got, `"secret_key":"s"`) {
		t.Errorf("missing secret_key: %s", got)
	}
}

func TestMergeIntoModel_PreservesWriteOnlyFields(t *testing.T) {
	// Klaviyo returns endpoint_url truncated and never returns
	// secret_key. mergeIntoModel must leave the model's endpoint_url
	// and secret_key alone.
	r := readResourceObject{
		ID: "01ABC",
		Attributes: attributes{
			Name:        "X",
			EndpointURL: "https://e", // truncated
			Enabled:     boolPtr(true),
		},
		Relationships: &readRelationships{WebhookTopics: &readTopicList{Data: []topicRef{
			{Type: "webhook-topic", ID: "event:klaviyo.sent_sms"},
		}}},
	}
	m := model{
		EndpointURL: types.StringValue("https://example.com/hook/full"),
		SecretKey:   types.StringValue("preserved-secret"),
	}
	mergeIntoModel(&m, r)
	if m.EndpointURL.ValueString() != "https://example.com/hook/full" {
		t.Errorf("endpoint_url got overwritten: %v", m.EndpointURL)
	}
	if m.SecretKey.ValueString() != "preserved-secret" {
		t.Errorf("secret_key got overwritten: %v", m.SecretKey)
	}
	if len(m.WebhookTopics) != 1 || m.WebhookTopics[0].ValueString() != "event:klaviyo.sent_sms" {
		t.Errorf("topics: %+v", m.WebhookTopics)
	}
}

func TestSend_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"type":"webhook","id":"01ABC","attributes":{
			"name":"X","endpoint_url":"https://e","enabled":true,
			"created_at":"2026-05-20T10:00:00+00:00"
		},"relationships":{"webhook-topics":{"data":[
			{"type":"webhook-topic","id":"event:klaviyo.sent_sms"}
		]}}}}`)
	}))
	defer srv.Close()

	r := newTestResource(t, srv.URL)
	body := writeEnvelope{Data: writeResourceObject{Type: apiType,
		Attributes:    attributes{Name: "X"},
		Relationships: &writeRelationships{},
	}}
	out, err := r.sendWrite(t.Context(), http.MethodPost, "/api/webhooks/", body, http.StatusCreated)
	if err != nil {
		t.Fatalf("sendWrite: %v", err)
	}
	if out.ID != "01ABC" {
		t.Errorf("decoded wrong: %+v", out)
	}
}

func boolPtr(b bool) *bool { return &b }

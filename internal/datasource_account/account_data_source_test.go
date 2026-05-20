package datasource_account

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"
)

func newTestDataSource(t *testing.T, baseURL string) *accountDataSource {
	t.Helper()
	c := client.New("test-key", "2026-04-15.pre", client.WithBaseURL(baseURL))
	return &accountDataSource{c: c}
}

func TestFetchFirst_ReturnsFirstAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/accounts/" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[
			{"type":"account","id":"AAA","attributes":{
				"test_account":false,"timezone":"UTC","preferred_currency":"USD",
				"public_api_key":"pub1","locale":"en-US","contact_information":{
					"default_sender_name":"x","default_sender_email":"x@y.com","organization_name":"X"
				}
			}}
		]}`)
	}))
	defer srv.Close()

	d := newTestDataSource(t, srv.URL)
	r, err := d.fetchFirst(t.Context())
	if err != nil {
		t.Fatalf("fetchFirst: %v", err)
	}
	if r.ID != "AAA" || r.Attributes.Timezone != "UTC" {
		t.Errorf("got: %+v", r)
	}
}

func TestFetchFirst_NoAccountsReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer srv.Close()

	d := newTestDataSource(t, srv.URL)
	_, err := d.fetchFirst(t.Context())
	if err == nil {
		t.Fatal("expected error on empty list")
	}
}

func TestFetchByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/accounts/BBB" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":{"type":"account","id":"BBB","attributes":{
			"test_account":true,"timezone":"US/Eastern","preferred_currency":"USD",
			"public_api_key":"pub2","locale":"en-US","contact_information":{
				"default_sender_name":"y","default_sender_email":"y@z.com","organization_name":"Y"
			}
		}}}`)
	}))
	defer srv.Close()

	d := newTestDataSource(t, srv.URL)
	r, err := d.fetchByID(t.Context(), "BBB")
	if err != nil {
		t.Fatalf("fetchByID: %v", err)
	}
	if r.ID != "BBB" || !r.Attributes.TestAccount {
		t.Errorf("got: %+v", r)
	}
}

func TestMergeIntoModel(t *testing.T) {
	industry := "Software"
	website := "https://aroma-zone.com"
	r := resourceObject{
		ID: "AAA",
		Attributes: attributes{
			TestAccount:       false,
			Industry:          &industry,
			Timezone:          "UTC",
			PreferredCurrency: "USD",
			PublicAPIKey:      "pub1",
			Locale:            "en-US",
			ContactInformation: &contactInformationDTO{
				DefaultSenderName:  "X",
				DefaultSenderEmail: "x@y.com",
				WebsiteURL:         &website,
				OrganizationName:   "X Inc",
			},
		},
	}
	var m model
	mergeIntoModel(&m, r)
	if m.ID.ValueString() != "AAA" || m.Industry.ValueString() != "Software" {
		t.Errorf("model = %+v", m)
	}
	if m.ContactInformation == nil || m.ContactInformation.WebsiteURL.ValueString() != "https://aroma-zone.com" {
		t.Errorf("contact_information: %+v", m.ContactInformation)
	}
}

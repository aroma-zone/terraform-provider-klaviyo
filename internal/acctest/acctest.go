// Package acctest holds the shared harness for this provider's
// acceptance tests.
//
// These are real tests: Terraform CLI drives the real provider binary
// against the real Klaviyo API. Nothing here is mocked. They only run
// when TF_ACC=1 (terraform-plugin-testing skips them otherwise) and
// require a working KLAVIYO_API_KEY.
//
// Because they mutate a live Klaviyo account, every object the suite
// creates is named with NamePrefix and a timestamp so it is obvious in
// the UI where it came from, and so a leaked object is easy to find and
// sweep. Each test cleans up after itself via CheckDestroy.
package acctest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/client"
	"github.com/aroma-zone/terraform-provider-klaviyo/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	// APIKeyEnvVar is the credential the acceptance suite needs.
	APIKeyEnvVar = "KLAVIYO_API_KEY"
	// RevisionEnvVar optionally overrides the pinned API revision.
	RevisionEnvVar = "KLAVIYO_API_REVISION"
	// BaseURLEnvVar optionally points the suite at a non-production
	// host (a sandbox account or a recording proxy).
	BaseURLEnvVar = "KLAVIYO_BASE_URL"

	// NamePrefix marks every object created by the acceptance suite.
	// Anything in the Klaviyo account carrying this prefix is test
	// debris and safe to delete.
	NamePrefix = "tfacc"
)

// ProviderFactories wires the in-process provider into Terraform over
// protocol v6. The version string is cosmetic; it only shows up in the
// User-Agent the client sends.
var ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"klaviyo": providerserver.NewProtocol6WithError(provider.New("acctest")()),
}

// PreCheck fails the test early, with an actionable message, when the
// credential is missing. It deliberately fails rather than skips: by
// the time PreCheck runs, TF_ACC=1 has already declared the intent to
// hit the real API, and a silent skip there reads as a green run.
func PreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv(APIKeyEnvVar) == "" {
		t.Fatalf("%s must be set for acceptance tests", APIKeyEnvVar)
	}
}

// APIClient returns a client built from the same environment the
// provider uses, for asserting server-side state out of band. Checks
// that go through this client are the ones that prove the provider
// actually created something in Klaviyo, as opposed to merely writing
// plausible values into Terraform state.
//
// It returns an error rather than taking a *testing.T because its main
// callers are resource.TestCheckFunc closures, which report failure by
// returning an error.
func APIClient() (*client.Client, error) {
	key := os.Getenv(APIKeyEnvVar)
	if key == "" {
		return nil, fmt.Errorf("%s must be set for acceptance tests", APIKeyEnvVar)
	}
	revision := os.Getenv(RevisionEnvVar)
	if revision == "" {
		revision = provider.DefaultAPIRevision
	}
	opts := []client.Option{client.WithUserAgent("terraform-provider-klaviyo/acctest")}
	if base := os.Getenv(BaseURLEnvVar); base != "" {
		opts = append(opts, client.WithBaseURL(base))
	}
	return client.New(key, revision, opts...), nil
}

// UniqueName builds a collision-proof, greppable name for a test
// object, e.g. "tfacc-flow-create-20260916T101500-8412".
func UniqueName(kind string) string {
	return fmt.Sprintf("%s-%s-%s-%04d",
		NamePrefix, kind, time.Now().UTC().Format("20060102T150405"), rand.Intn(10000))
}

// GetAttributes fetches a JSON:API object out of band and returns the
// HTTP status alongside its `data.attributes` map. A 404 comes back as
// (404, nil, nil) with no error so callers can assert on absence.
func GetAttributes(c *client.Client, path string) (int, map[string]any, error) {
	resp, err := c.Do(context.Background(), http.MethodGet, path, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("out-of-band GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, fmt.Errorf("out-of-band GET %s: unexpected status %d: %w",
			path, resp.StatusCode, client.DecodeError(resp))
	}

	var env struct {
		Data struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return resp.StatusCode, nil, fmt.Errorf("decoding out-of-band GET %s: %w", path, err)
	}
	return resp.StatusCode, env.Data.Attributes, nil
}

// Deleted reports whether the object at path is gone from Klaviyo.
func Deleted(c *client.Client, path string) (bool, error) {
	status, _, err := GetAttributes(c, path)
	if err != nil {
		return false, err
	}
	return status == http.StatusNotFound, nil
}

// DeleteOutOfBand removes an object behind Terraform's back, to
// simulate someone deleting it in the Klaviyo UI.
func DeleteOutOfBand(c *client.Client, path string) error {
	resp, err := c.Do(context.Background(), http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("out-of-band DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	default:
		return fmt.Errorf("out-of-band DELETE %s: unexpected status %d: %w",
			path, resp.StatusCode, client.DecodeError(resp))
	}
}

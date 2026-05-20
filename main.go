// Binary terraform-provider-klaviyo is the entrypoint Terraform calls when
// the provider is invoked. It hands a Plugin Framework provider to
// `providerserver.Serve`, which speaks the Terraform plugin protocol.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/aroma-zone/terraform-provider-klaviyo/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is overridden at link time by GoReleaser via -ldflags.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(
		context.Background(),
		provider.New(version),
		providerserver.ServeOpts{
			Address: "registry.terraform.io/aroma-zone/klaviyo",
			Debug:   debug,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}

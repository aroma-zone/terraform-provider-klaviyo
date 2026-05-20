# terraform-provider-klaviyo

Terraform provider for [Klaviyo](https://www.klaviyo.com), built from
Klaviyo's official [OpenAPI specification](https://github.com/klaviyo/openapi)
using HashiCorp's
[`terraform-plugin-codegen-openapi`](https://github.com/hashicorp/terraform-plugin-codegen-openapi)
and
[`terraform-plugin-codegen-framework`](https://github.com/hashicorp/terraform-plugin-codegen-framework).

> **Status:** in development. v0.1.0 will expose a single resource —
> `klaviyo_list` — to prove the pipeline. More resources land after that.

## Source

This provider's source lives in the
[`aroma-zone/terraform-providers`](https://github.com/aroma-zone/terraform-providers)
private monorepo under `providers/klaviyo/`. Each release is mirrored to
this repo automatically; **do not open pull requests here**, open them
against the monorepo.

## Versioning

We pin to a specific Klaviyo API revision (see [`spec/version.txt`](./spec/version.txt)).
Bumping the revision is a manual, reviewable change: edit `spec/version.txt`,
run `make fetch-spec generate`, review the diff, commit.

| Field | Value |
|---|---|
| Klaviyo API revision | `2026-04-15` |
| Upstream commit | [`796408b`](https://github.com/klaviyo/openapi/tree/796408bfb8bb1ea6f27d730ae548e492bfc0c5b0) |

## Building locally

```sh
make tools          # install codegen toolchain into ./bin
make generate       # OpenAPI -> code spec -> Go scaffolding
make build          # produce ./bin/terraform-provider-klaviyo
make test           # run unit tests
```

`make help` lists all targets.

## License

[MPL-2.0](../../LICENSE) — same as the upstream Terraform provider ecosystem.

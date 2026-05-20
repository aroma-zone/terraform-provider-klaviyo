# terraform-provider-klaviyo

Terraform provider for [Klaviyo](https://www.klaviyo.com).

> **Status:** in development. v0.1.0 will expose a single resource —
> `klaviyo_list` — to prove the pipeline. More resources land after that.

## Source

This provider's source lives in the
[`aroma-zone/terraform-providers`](https://github.com/aroma-zone/terraform-providers)
private monorepo under `providers/klaviyo/`. Each release is mirrored to
this repo automatically; **do not open pull requests here**, open them
against the monorepo.

## How this provider is built

The provider is **hand-written** against Klaviyo's
[OpenAPI specification](https://github.com/klaviyo/openapi). We pin the
spec into [`spec/openapi.json`](./spec/openapi.json) so it's the source
of truth for field names, types, and semantics — but the Terraform
schema and CRUD code is human-authored to keep the HCL surface clean
(Klaviyo's API uses JSON:API conventions which do not map cleanly onto
Terraform resources). HashiCorp's `tfplugingen-openapi` was evaluated
and rejected because it cannot decompose `allOf` schemas and would have
leaked the JSON:API envelope into the user-facing HCL.

| | |
|---|---|
| Klaviyo API revision | `2026-04-15` |
| Upstream commit | [`796408b`](https://github.com/klaviyo/openapi/tree/796408bfb8bb1ea6f27d730ae548e492bfc0c5b0) |
| Plugin Framework version | `v1.19.0` |
| Minimum Go version | `1.25` |

## Building locally

```sh
make build        # produce ./bin/terraform-provider-klaviyo
make test         # run unit tests
make tools docs   # regenerate ./docs from the schema (tfplugindocs)
make fetch-spec   # refresh spec/openapi.json from the pinned upstream commit
```

`make help` lists all targets.

## Bumping the upstream spec

1. Edit `spec/version.txt` (set the new revision + commit SHA).
2. Run `make fetch-spec`.
3. Inspect the diff in `spec/openapi.json` for breaking changes.
4. Update the hand-written schema / CRUD where the API changed.
5. Bump the provider version in [Pulumi.yaml or .goreleaser.yml] and tag a release.

## License

[MPL-2.0](../../LICENSE) — same license HashiCorp's first-party Terraform
providers use.

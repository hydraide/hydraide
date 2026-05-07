# Contributor setup for the Go SDK

This page is for people contributing to HydrAIDE itself. If you only want to **use** the SDK in your own project, see [`install.md`](install.md) instead.

## The monorepo is multi-module

Two Go modules live in this repository:

| Path | Module | What it contains |
|---|---|---|
| `./` | `github.com/hydraide/hydraide` | Server, hydraidectl, examples, e2e tests |
| `./sdk/go/hydraidego/` | `github.com/hydraide/hydraide/sdk/go/hydraidego/v3` | The Go SDK and the generated proto stubs (the `/v3` suffix is required by Go's semantic import versioning for major versions ≥ 2) |

The two modules are tied together by `go.work` at the repo root, which lets you edit both modules in one checkout and have them resolve each other locally.

The root `go.mod` carries a `replace` directive that points the SDK module path at the in-tree path:

```
replace github.com/hydraide/hydraide/sdk/go/hydraidego/v3 => ./sdk/go/hydraidego
```

This means a build of the server or hydraidectl from this checkout always uses the in-tree SDK source, not whatever version the Go proxy serves. External consumers of the SDK never see this `replace`; the published module on the proxy is self-contained.

## Day-to-day commands

```bash
# Build everything from the repo root
go build ./...

# Build only the SDK module
cd sdk/go/hydraidego && go build ./...

# Run all unit tests across both modules
go test -short ./...

# Run e2e tests (require a running HydrAIDE test instance + env vars)
go test -tags=e2e ./app/server/e2etests/...

# Regenerate proto stubs after editing proto/hydraide.proto
make proto-go
```

## When you add a dependency

If the SDK code adds a new third-party dependency:

```bash
cd sdk/go/hydraidego
go get example.com/new/dep@latest
go mod tidy
cd ../../..
go mod tidy   # the parent module's go.sum may also need updating
```

If the parent (server / hydraidectl) adds a dependency, only the root `go.mod` needs `tidy`.

Keep the two `go.mod` files in sync on shared dependencies (gRPC, protobuf, msgpack). Mismatched versions sometimes work via `go.work`, but they break the published SDK or the server binary in non-obvious ways.

## Releasing the SDK

The SDK ships under tags shaped like `sdk/go/hydraidego/vX.Y.Z`. Pushing such a tag triggers `.github/workflows/cd_sdk_go.yaml`, which:

1. Builds the SDK module.
2. Runs unit tests.
3. Creates a GitHub Release with install instructions.

A push to the proxy (the publicly-visible `@latest` resolution) happens automatically the first time anyone runs `go get …@vX.Y.Z`; the release workflow does not push to the proxy directly.

To cut a release:

```bash
git tag -a sdk/go/hydraidego/v3.X.Y -m "Go SDK v3.X.Y: <one-line summary>"
git push origin sdk/go/hydraidego/v3.X.Y
```

Match the major to the server era. Do not move existing tags; if you need to fix a release, cut the next patch.

## Why split the SDK out

Before the split, the entire monorepo was one Go module. That meant `go get github.com/hydraide/hydraide/sdk/go/hydraidego/v3@latest` did not work cleanly (the existing `v2.0.7` root tag violated semantic import versioning since the module path lacked `/v2`), and consumers pulled the whole monorepo into their dependency graph. Splitting fixes both: `@latest` now resolves to the highest `sdk/go/hydraidego/v3.x.y` tag, and the SDK module ships only its own runtime dependencies. The SDK module path itself carries the `/v3` major-version suffix, as Go requires for any module at major version ≥ 2.

## Common gotchas

- **`gopls` confused by `go.work`**: most modern versions handle workspaces fine, but if your IDE shows phantom errors, restart `gopls` after pulling.
- **`go mod tidy` complains in the SDK module about parent imports**: this should never happen now (the SDK has zero parent dependency). If it does, an e2e or test file leaked back into the SDK package; move it to `app/server/e2etests/`.
- **Forgetting to bump the SDK tag after a server major bump**: server users on the new major need a matching SDK major, or filter / patch behaviour can drift silently.

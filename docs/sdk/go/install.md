# Installing and upgrading the Go SDK

The HydrAIDE Go SDK lives in its own Go module inside the monorepo:

```
github.com/hydraide/hydraide/sdk/go/hydraidego/v3
```

Pulling the SDK does **not** drag the rest of the monorepo (server, hydraidectl, examples) into your dependency graph. Only the SDK module's own dependencies (gRPC, protobuf, msgpack, xxhash) end up in your `go.sum`.

## Install

In an existing Go module:

```bash
go get github.com/hydraide/hydraide/sdk/go/hydraidego/v3@latest
```

Pinning to a specific version:

```bash
go get github.com/hydraide/hydraide/sdk/go/hydraidego/v3@v3.0.1
```

In a fresh project:

```bash
mkdir myapp && cd myapp
go mod init github.com/you/myapp
go get github.com/hydraide/hydraide/sdk/go/hydraidego/v3@latest
```

After install, import it:

```go
import (
    "github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
    "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
    "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/hydraidehelper"
    "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
)
```

## Upgrade

```bash
go get -u github.com/hydraide/hydraide/sdk/go/hydraidego/v3@latest
go mod tidy
```

To check the version you currently use:

```bash
go list -m github.com/hydraide/hydraide/sdk/go/hydraidego/v3
```

To see all available versions:

```bash
go list -m -versions github.com/hydraide/hydraide/sdk/go/hydraidego/v3
```

The `@latest` query resolves to the highest semver tag on the proxy. There is no separate moving "latest" tag to maintain; `@latest` is computed automatically.

## Version compatibility

| Server | Compatible Go SDK |
|---|---|
| `server/v3.x` | `sdk/go/hydraidego/v3.x` |

The major version of the SDK matches the major version of the server era it targets. Minor and patch versions move independently. A v3 SDK can talk to any v3 server release; cross-major compatibility is not guaranteed.

If you upgrade the server to a new major (e.g. v3 to v4), bump the SDK major in lockstep.

## Module layout note

This repository is a multi-module Go monorepo. If you contribute to HydrAIDE itself (vs. consuming the SDK as a third party), see [`docs/sdk/go/contributor-setup.md`](contributor-setup.md) for the workspace setup with `go.work` and the `replace` directive.

## Claude Code companion

If you use [Claude Code](https://claude.com/claude-code), install the HydrAIDE plugin to get three skills (Go SDK reference, server operations, conceptual explanations) and three slash commands (`/hydraide-new-model`, `/hydraide-review`, `/hydraide-debug`):

```
/plugin marketplace add hydraide/claude
/plugin install hydraide
```

See [`docs/claude-friendly.md`](../../claude-friendly.md) for what the plugin actually does.

## Troubleshooting

**`go: module github.com/hydraide/hydraide/sdk/go/hydraidego/v3: reading … 410 Gone`**: the proxy has not yet cached the version. Wait a minute, or run `GOPROXY=direct go get …` once to bypass the proxy.

**`ambiguous import: found package … in multiple modules`**: your project has a `go.work` that pulls in both the parent `hydraide/hydraide` and the SDK, or a stale `replace` directive. Remove the parent module from your workspace, or drop the `replace` if you do not need a local checkout.

**`unknown revision`**: the tag has not propagated yet. Check available tags with `git ls-remote --tags https://github.com/hydraide/hydraide | grep sdk/go/hydraidego`.

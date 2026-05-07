---
name: hydraide-data-ops
description: Ad-hoc data operation CLIs against a HydrAIDE instance. Migration between swamps, restore from export, bulk import, bulk delete, bulk update, orphan cleanup, cross-environment data sync, reconciliation. Use when the user asks Claude to write a one-shot Go program that reads, transforms, writes, or removes data in HydrAIDE outside of long-lived application code. For long-lived application logic, use `hydraidego`. For binary-level server operations (backup the instance, upgrade the binary, restart, observe), use `hydraidectl`.
---

# HydrAIDE Data Ops

This skill is for the case where the user asks Claude to write a quick Go CLI that reads, transforms, writes, or removes data in a HydrAIDE instance. Migration scripts, bulk imports, mass deletes, orphan finders, restore-from-export tools, cross-environment data sync, reconciliation jobs. Code that exists for one purpose, runs a few times, and is checked in for traceability.

The output of this skill is **always a Go file under `cmd/<task-name>/main.go`** (or the project's equivalent CLI directory) that follows the safety rails below. Activate the `hydraidego` skill alongside this one for SDK API details (filters, batch ops, struct tags).

## When to use this skill vs. its siblings

| User's intent | Skill |
|---|---|
| "Write a one-shot script that migrates / imports / cleans up / restores data" | **`hydraide-data-ops`** (this skill) |
| "Write a service / API handler / model" (long-lived application code) | `hydraidego` |
| "Backup the whole instance / restart the server / observe / migrate V1 storage to V2 / install hydraidectl" | `hydraidectl` |
| "Install HydrAIDE / upgrade the server or SDK / bootstrap" | `hydraide-install-and-upgrade` |

## Step 0: backup posture (ALWAYS, before anything else)

Before writing a single line of destructive code, confirm the backup situation. This step is non-negotiable on live data; for test or staging it is a strong recommendation.

Ask the user, before the requirements batch:

- Do you have a recent backup of the source Swamp(s) this script will touch?
- When was it taken? Where is it stored? Has the restore been verified (test-restored at least once)?
- If no backup exists, do you want help producing one before this script runs?

Decision rules:

| Op type and env | Backup posture |
|---|---|
| Read-only ops (orphan finders, reconciliation reports) | Backup not required, proceed. |
| Destructive ops on `test` or `staging` | Backup recommended. If the user accepts the risk explicitly, proceed. |
| Destructive ops on `live` | Backup REQUIRED. Refuse to proceed without an explicit confirmation that a recent verified backup exists. |

If the user wants help producing a backup, hand off to the `hydraidectl` skill (it covers `hydraidectl backup`, the restore-verify procedure, and storage placement). Once the backup is confirmed and verified, return to this skill and continue with Step 1.

A 30-second backup check has saved more production recoveries than any cleverness inside the migration script.

## Step 1: requirements (batched AskUserQuestion)

Once backup posture is settled, ask the rest in one tool call:

1. **Source and target Swamps**: Sanctuary/Realm/Swamp identifiers for what to read from and what to write to. Same instance or different instances?
2. **Model struct**: does the target struct already exist in the project, or does this CLI need a fresh minimal struct definition?
3. **Environment first**: should we run on `test` (or `staging`) before `live`? Default yes; refuse to skip without an explicit reason.
4. **Filter scope**: full sweep, or a subset (per tenant, per ID, per time window)?
5. **Idempotent re-run**: must the CLI produce the same result if run twice, or is one-shot acceptable?
6. **Verify and destroy**: for migrations, should the source be destroyed automatically after a verify pass, or left in place for manual review?

If the user cannot answer one of these, the script is not ready to write yet. Push back; do not guess.

## The standard CLI skeleton

Every `hydraide-data-ops` script follows the same shape. Generate this skeleton, then fill in the task-specific logic.

```go
// Package main: <one-line task description>.
//
// Usage:
//
//	# DRY-RUN against the test environment (reads only, never writes)
//	cd <module-root> && APP_ENV=test go run ./cmd/<task-name>
//
//	# LIVE-RUN against the test environment (actually writes)
//	cd <module-root> && APP_ENV=test go run ./cmd/<task-name> --live-run
//
//	# LIVE-RUN scoped to a single entity
//	cd <module-root> && APP_ENV=test go run ./cmd/<task-name> --live-run --id=<id>
//
//	# LIVE-RUN against the live environment (extra confirmation required)
//	cd <module-root> && APP_ENV=live go run ./cmd/<task-name> --live-run --confirm=yes
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
)

func main() {
	liveRun := flag.Bool("live-run", false, "PERFORM WRITES. Without this flag the script is a dry run and never writes.")
	confirm := flag.String("confirm", "", "Required on live env when --live-run is set: --confirm=yes")
	idFilter := flag.String("id", "", "Optional: limit the run to one entity by its key")
	flag.Parse()

	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		slog.Error("APP_ENV is empty. Set APP_ENV=test|staging|live before running.")
		os.Exit(1)
	}

	slog.Info("data op starting",
		"env", env, "liveRun", *liveRun, "idFilter", *idFilter)

	// Live env protection: require explicit --confirm=yes for writes.
	if env == "live" && *liveRun && *confirm != "yes" {
		fmt.Println()
		fmt.Println("  Refusing: --live-run on live env requires --confirm=yes")
		os.Exit(1)
	}

	// Interactive confirmation for any write run.
	if *liveRun {
		fmt.Printf("  About to run with WRITES on env=%q. Continue? (yes/no) ", env)
		var ans string
		fmt.Scanln(&ans)
		if strings.ToLower(strings.TrimSpace(ans)) != "yes" {
			fmt.Println("  Cancelled.")
			os.Exit(0)
		}
	}

	r := buildRepo(env)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := run(ctx, r, *liveRun, *idFilter); err != nil {
		slog.Error("data op failed", "error", err)
		os.Exit(1)
	}
	slog.Info("data op finished")
}

func buildRepo(env string) repo.Repo {
	// Cert paths are project-specific. Read them from APP_<ENV>_CERT_DIR or
	// equivalent env vars; never hard-code per-environment hosts in the CLI.
	host := os.Getenv("APP_" + strings.ToUpper(env) + "_HYDRA_HOST")
	certDir := os.Getenv("APP_" + strings.ToUpper(env) + "_HYDRA_CERT_DIR")
	if host == "" || certDir == "" {
		slog.Error("missing connection env vars", "env", env)
		os.Exit(1)
	}
	return repo.New([]*client.Server{{
		Host:          host,
		FromIsland:    1,
		ToIsland:      1000,
		CACrtPath:     certDir + "/ca.crt",
		ClientCrtPath: certDir + "/client.crt",
		ClientKeyPath: certDir + "/client.key",
	}}, 1000, 10737418240, false)
}

func run(ctx context.Context, r repo.Repo, liveRun bool, idFilter string) error {
	h := r.GetHydraidego()
	// task-specific body goes here
	_ = h
	return nil
}
```

The skeleton is non-negotiable on flags, env handling, dry-run default, and live-env protection. Adjust the task body, not the safety frame.

## Task templates (fill in the `run(...)` body)

### 1. Migration: source Swamp -> target Swamp

```go
// Read every record from the source, transform, write to target, verify, optionally destroy source.
sourceSwamp := name.New().Sanctuary("app").Realm("source-realm").Swamp("scope")
targetSwamp := name.New().Sanctuary("app").Realm("target-realm").Swamp("scope")

var migrated, skipped, failed int

err := h.CatalogReadMany(ctx, sourceSwamp,
	&hydraidego.Index{IndexType: hydraidego.IndexCreationTime, IndexOrder: hydraidego.IndexOrderAsc},
	SourceModel{},
	func(model any) error {
		src := model.(*SourceModel)

		// Optional filter scope.
		if idFilter != "" && src.Key != idFilter {
			skipped++
			return nil
		}

		// Transform.
		dst := transform(src)
		// Always set CreatedAt for catalog models with a `createdAt` tag.
		dst.CreatedAt = time.Now().UTC()

		if !liveRun {
			slog.Info("DRY: would migrate", "key", src.Key)
			migrated++
			return nil
		}

		if _, err := h.CatalogSave(ctx, targetSwamp, dst); err != nil {
			slog.Error("migrate failed", "key", src.Key, "error", err)
			failed++
			return nil // continue on per-record errors
		}
		migrated++
		return nil
	})
if err != nil && !hydraidego.IsSwampNotFound(err) && !hydraidego.IsNotFound(err) {
	return err
}

slog.Info("migration complete", "migrated", migrated, "skipped", skipped, "failed", failed)

// Verify: count target == count source (minus skipped).
// Only destroy source after a clean verify and only on --live-run --destroy-old.
```

### 2. Bulk delete by filter

Use `CatalogReadManyStream` with a server-side filter to collect matching keys (KeysOnly mode for speed), then `CatalogDeleteMany` in batches. Never loop with single `CatalogDelete` calls.

```go
var keysToDelete []string
err := h.CatalogReadManyStream(ctx, swamp,
	&hydraidego.Index{KeysOnly: true, MaxResults: 10000},
	hydraidego.FilterAND(
		hydraidego.FilterBytesFieldBool(hydraidego.Equal, "Archived", true),
		hydraidego.FilterBytesFieldTime(hydraidego.LessThan, "UpdatedAt", time.Now().Add(-180*24*time.Hour)),
	),
	Model{},
	func(model any) error {
		keysToDelete = append(keysToDelete, model.(*Model).Key)
		return nil
	})
// chunk and delete in batches of e.g. 500 keys per CatalogDeleteMany call.
```

### 3. Bulk import from external source

Read CSV/JSON/protobuf, batch-build models with `CreatedAt = time.Now().UTC()` set, call `CatalogSaveMany` (single Swamp) or `CatalogSaveManyToMany` (across many Swamps).

### 4. Orphan finder (cross-Swamp set diff)

Load both Swamps in `KeysOnly` mode into Go sets, compute the difference, write the result to a file (read-only by design; never delete unless this is paired with a follow-up cleanup script).

### 5. Bulk update (rewrite a field)

Prefer `CatalogPatchFieldsMany` over read-modify-write. Patch is atomic per key, supports conditions, and avoids races. If a transformation is too complex for a patch op, fall back to read + transform + `CatalogSave`, but wrap each batch in a `Lock`/`Unlock` if there is any chance of concurrent writers.

## Connection setup notes

- Hard-coded hosts and cert paths in the CLI are a smell. Read them from env vars per environment.
- For multi-instance ops (read from instance A, write to instance B), build two `repo.Repo` values, name them `srcRepo`, `dstRepo`. Pass both into `run(ctx, srcRepo, dstRepo, ...)`.
- The `repo.New(servers, maxMessageSize, blockSize, useTLS)` constants for `maxMessageSize` and `blockSize` follow project conventions; copy from the long-lived service in the same module rather than inventing values.

## Safety rails (non-negotiable)

- **Dry-run is the default.** `--live-run` is the only switch that enables writes. The script must clearly log "DRY:" prefixes when in dry-run.
- **Live env requires `--confirm=yes`.** Refuse to write on `APP_ENV=live` without it.
- **Interactive confirmation on every write run.** A single `Scanln` for "yes" is enough; refuse on anything else.
- **Structured logging only.** `slog.Info` / `slog.Error` with key-value fields; never `fmt.Println` for diagnostic output (keep `Println` for the interactive confirmation banner only).
- **Use batch ops, never loops over single-key calls.** `CatalogReadBatch`, `CatalogSaveMany`, `CatalogDeleteMany`, `AreKeysExist`, `ProfileReadBatch`, etc.
- **Set `CreatedAt = time.Now().UTC()` for any new Catalog write** that uses a `createdAt` tag; the server silently drops the write otherwise.
- **Context with timeout.** A long migration deserves a 30 minute or 1 hour timeout, not the default 30 seconds.
- **Verify before destroy.** Migrations always read back and count before any destructive cleanup.

## Common pitfalls in data-ops scripts

- **Forgetting `--live-run` to be required for writes**: trivial to slip in a `Save` outside the gate. Audit every write call site in the script.
- **Hardcoded `live` host even when env=`test`**: copy/paste from a sibling CLI introduces this; always read from env vars.
- **Loop of single calls**: 100k iterations of `CatalogRead` instead of `CatalogReadBatch` is a 1000x slowdown. Refuse to write that, even if it works.
- **Zero `CreatedAt` on save**: silent drop. Run a small verify after the first batch to catch this immediately.
- **No idempotency check on re-run**: if the script crashes halfway through and is rerun, do not assume "starting fresh". Either skip already-migrated records (check existence in target first), or document explicitly that the script is one-shot only.
- **Concurrent writers on the same key during migration**: if the long-lived application is still writing to the source while you migrate, use a `Lock`/`Unlock` per key, or quiesce the producer first.
- **Filter syntax mistakes silently returning empty**: a `CatalogReadManyStream` with a typo'd filter field name returns zero rows without error. Always run dry-run first and verify the row count matches expectations.

## Output format

When generating a CLI for the user:

1. Confirm the answers from the question batch above. Do not proceed with one missing.
2. Print a short plan (source, target, transformation, dry-run output expectations, write expectations).
3. Generate the file at `cmd/<task-name>/main.go`. The first line of the file is a `// Package main:` comment that summarises the task, followed by the usage block.
4. List the env vars the script expects (`APP_ENV`, `APP_TEST_HYDRA_HOST`, etc.).
5. Hand off the next step to the user: "run the dry-run on the test env first, paste the output back, then we decide on `--live-run`".

## What this skill is not

- **Not a substitute for `hydraidectl backup`.** A full instance backup is operational tooling, not a data-ops script.
- **Not a place for application logic.** If the script gets longer than a few hundred lines, or grows shared types and helpers, the work probably belongs in the long-lived service, not in `cmd/<task-name>/`.
- **Not a place to skip safety because the user is in a hurry.** Push back. The 5 minutes spent on a dry-run save days of recovery.

# HydrAIDE Migration Guide: V1 to V2 Storage Engine

## Overview

HydrAIDE 3.0 introduces the **V2 Storage Engine**, a completely redesigned append-only storage format that delivers significant performance improvements:

| Metric | V1 | V2 | Improvement |
|--------|----|----|-------------|
| **Insert 100K entries** | 1274ms | 40ms | **32x faster** |
| **Update 10K entries** | 195ms | 4ms | **49x faster** |
| **Delete 10K entries** | 191ms | 1.7ms | **112x faster** |
| **Read 100K entries** | 4005ms | 79ms | **51x faster** |
| **Files per swamp** | 21-23 | 1 | **95% fewer** |
| **Storage size** | 3.0 MB | 1.5 MB | **50% smaller** |

---

## ⚠️ IMPORTANT: Migration Checklist

Before migrating, ensure:

1. ✅ You have the latest `hydraidectl` installed
2. ✅ You have the latest HydrAIDE server version
3. ✅ You have created a backup of your data
4. ✅ The HydrAIDE server is **STOPPED**

---

## Complete Migration Procedure

Follow these steps in order for a safe and successful migration:

### Step 1: Check for hydraidectl Updates

First, make sure you have the latest hydraidectl:

```bash
hydraidectl version
```

If an update is available, update hydraidectl:

```bash
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash
```

### Step 2: Stop the HydrAIDE Server

Stop the instance to ensure no data is being written during migration:

```bash
sudo hydraidectl stop --instance <your-instance-name>
```

Example:
```bash
sudo hydraidectl stop --instance prod
```

Wait for the graceful shutdown to complete. This ensures all in-memory data is flushed to disk.

### Step 3: Create a Backup

**This step is critical!** Create a compressed backup of your data:

```bash
sudo hydraidectl backup --instance <your-instance-name> --output /path/to/backup --compress
```

Example:
```bash
sudo hydraidectl backup --instance prod --output /backup/hydraide --compress
```

This creates a compressed backup that can be restored if anything goes wrong.

### Step 4: Update the HydrAIDE Server (Without Starting)

Update to the latest server version, but do NOT start the server yet:

```bash
sudo hydraidectl update --instance <your-instance-name> --no-start
```

Example:
```bash
sudo hydraidectl update --instance prod --no-start
```

The `--no-start` flag ensures the server stays stopped so you can run the migration.

### Step 5: Run the Migration

Now run the actual migration with full mode:

```bash
sudo hydraidectl migrate --instance <your-instance-name> --full
```

Example:
```bash
sudo hydraidectl migrate --instance prod --full
```

The `--full` flag will:
- ✅ Migrate all V1 swamps to V2 format
- ✅ Verify data integrity after migration
- ✅ Delete old V1 files (after successful migration)
- ✅ Set the storage engine to V2

**Note:** The server will NOT be started automatically after migration, giving you control over when to start it.

### Step 6: Verify Migration Results

Check the migration output for:
- Number of swamps migrated
- Any errors or warnings
- Size savings achieved

If there were any errors, check the specific swamps and fix the issues before continuing.

### Step 7: Start the HydrAIDE Server

Once you've verified the migration was successful, start the server:

```bash
sudo hydraidectl start --instance <your-instance-name>
```

Example:
```bash
sudo hydraidectl start --instance prod
```

### Step 8: Verify Server Health

Check that the server is running correctly:

```bash
hydraidectl health --instance <your-instance-name>
```

Test your application to ensure everything works as expected.

---

## Quick Reference: Complete Command Sequence

```bash
# 1. Check for updates
hydraidectl version

# 2. Update hydraidectl if needed
curl -sSfL https://raw.githubusercontent.com/hydraide/hydraide/main/scripts/install-hydraidectl.sh | bash

# 3. Stop the server
sudo hydraidectl stop --instance prod

# 4. Create backup
sudo hydraidectl backup --instance prod --output /backup/hydraide --compress

# 5. Update server without starting
sudo hydraidectl update --instance prod --no-start

# 6. Run migration
sudo hydraidectl migrate --instance prod --full

# 7. Verify results (check output above)

# 8. Start the server
sudo hydraidectl start --instance prod

# 9. Verify health
hydraidectl health --instance prod
```

---

## Migration Options

### Dry Run (Recommended First Step)

Run a dry-run to see what would be migrated without making any changes:

```bash
hydraidectl migrate --instance prod --dry-run
```

### Parallel Migration (Faster for Large Datasets)

For large datasets, use multiple worker threads:

```bash
hydraidectl migrate --instance prod --full --parallel 8
```

### JSON Output

For scripting or automation:

```bash
hydraidectl migrate --instance prod --full --json
```

---

## Command Options Reference

| Option | Description | Default |
|--------|-------------|---------|
| `--instance`, `-i` | Instance name | **Required** |
| `--data-path` | Direct path to data directory (alternative to --instance) | - |
| `--full` | Complete migration (migrate + set engine + cleanup) | `false` |
| `--dry-run` | Simulate migration without changes | `false` |
| `--parallel` | Number of parallel workers | `4` |
| `--verify` | Verify data after migration | `true` (with --full) |
| `--delete-old` | Delete original V1 files | `true` (with --full) |
| `--json` | Output results as JSON | `false` |

---

## Migration Output Example

```
HydrAIDE V1 → V2 Migration
==========================

Instance: prod
Data path: /var/hydraide/prod/data
Mode: LIVE MIGRATION
Workers: 4

Scanning for V1 swamps...
Found: 15,234 V1 swamps

Migrating...
[████████████████████████████████] 100% (15234/15234)

Migration Complete!
===================
✅ Migrated: 15,234 swamps
❌ Errors: 0
📁 Size before: 45.2 GB
📁 Size after: 23.1 GB
💾 Saved: 22.1 GB (49%)
⏱️ Duration: 4m 32s

Setting engine to V2...
✅ Engine set to V2

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🎉 Migration completed successfully!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

The instance was NOT started automatically.
Please verify the migration results above, then start the server manually:
  sudo hydraidectl start --instance prod
```

---

## Troubleshooting

### Migration Fails Midway

If migration fails partway through:

1. Check error messages in the output
2. Fix the underlying issue (disk space, permissions, etc.)
3. Re-run the migration - it will skip already-migrated swamps

### Rollback to V1

If you need to rollback:

1. Stop HydrAIDE (if running)
2. Restore from your backup:
   ```bash
   sudo hydraidectl restore --instance prod --source /backup/hydraide/prod-backup.tar.gz
   ```
3. Set engine back to V1:
   ```bash
   hydraidectl engine --instance prod --set v1
   ```
4. Start HydrAIDE:
   ```bash
   sudo hydraidectl start --instance prod
   ```

### Verification Errors

If verification reports mismatches:

1. Check the specific swamps reported
2. Re-run migration for those swamps
3. If issues persist, restore from backup and report the issue

### Not Enough Disk Space

The migration temporarily needs extra disk space. Solutions:

1. Use `--delete-old` to delete V1 files immediately after each swamp migrates
2. Free up disk space before migration
3. Migrate in batches

---

## Questions?

- 📚 [HydrAIDE Documentation](../README.md)
- 💬 [Join Discord](https://discord.gg/xE2YSkzFRm)
- 🐛 [Report Issues](https://github.com/hydraide/hydraide/issues)

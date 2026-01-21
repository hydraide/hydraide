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

## ⚠️ IMPORTANT: Backup Before Migration

> **Always create a complete backup of your HydrAIDE data directory before starting any migration!**

```bash
# Example backup command
cp -r /path/to/hydraide/data /path/to/backup/hydraide-data-$(date +%Y%m%d)
```

The migration process modifies your data files. While the migrator includes verification steps, having a backup ensures you can recover if anything goes wrong.

---

## Migration Commands

### 1. Dry Run (Recommended First Step)

Run a dry-run to see what would be migrated without making any changes:

```bash
hydraidectl migrate --source /path/to/hydraide/data --dry-run
```

This will:
- Scan all directories for V1 swamps
- Report how many swamps would be migrated
- Show estimated size savings
- **Not modify any files**

### 2. Full Migration

Once you've verified the dry-run results, run the actual migration:

```bash
hydraidectl migrate --source /path/to/hydraide/data
```

### 3. Parallel Migration (Faster)

For large datasets, use multiple worker threads:

```bash
hydraidectl migrate --source /path/to/hydraide/data --workers 8
```

### 4. Migration with Verification

Enable verification to ensure data integrity after migration:

```bash
hydraidectl migrate --source /path/to/hydraide/data --verify
```

### 5. Keep Original Files (Safe Mode)

Keep original V1 files after migration (uses more disk space):

```bash
hydraidectl migrate --source /path/to/hydraide/data --keep-original
```

---

## Command Options

| Option | Description | Default |
|--------|-------------|---------|
| `--source`, `-s` | Path to HydrAIDE data directory | **Required** |
| `--dry-run`, `-d` | Simulate migration without changes | `false` |
| `--workers`, `-w` | Number of parallel workers | `4` |
| `--verify`, `-v` | Verify data after migration | `false` |
| `--keep-original` | Don't delete original V1 files | `false` |

---

## What the Migrator Does

1. **Scans** the source directory recursively for V1 swamps
2. **Detects** V1 format (multiple chunk files + meta.json)
3. **Reads** all data from V1 chunks
4. **Writes** data to new V2 `.hyd` file format
5. **Verifies** (if enabled) that all data was migrated correctly
6. **Removes** (unless `--keep-original`) old V1 files
7. **Reports** statistics (swamps migrated, size saved, errors)

---

## Migration Output Example

```
HydrAIDE V1 → V2 Migration
==========================

Source: /data/hydraide
Mode: Live Migration
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
```

---

## Post-Migration

After migration:

1. **Verify your application works correctly** with the migrated data
2. **Monitor performance** - you should see significant improvements
3. **Update your configuration** to use `UseChroniclerV2: true` for new swamps
4. **Remove backups** once you're confident the migration was successful

---

## Troubleshooting

### Migration Fails Midway

If migration fails partway through:

1. Check error messages in the output
2. Fix the underlying issue (disk space, permissions, etc.)
3. Re-run the migration - it will skip already-migrated swamps

### Rollback to V1

If you need to rollback:

1. Stop HydrAIDE
2. Restore from your backup
3. Restart HydrAIDE

### Verification Errors

If verification reports mismatches:

1. Check the specific swamps reported
2. Re-run migration for those swamps
3. If issues persist, restore from backup and report the issue

---

## Questions?

- 📚 [HydrAIDE Documentation](../README.md)
- 💬 [Join Discord](https://discord.gg/xE2YSkzFRm)
- 🐛 [Report Issues](https://github.com/hydraide/hydraide/issues)

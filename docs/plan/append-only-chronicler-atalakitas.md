# HydrAIDE Chronicler Átalakítás: Append-Only Block Storage

**Dátum:** 2026-01-21  
**Verzió:** 1.0  
**Státusz:** Tervezet - Jóváhagyásra vár

---

## 1. Összefoglaló

### Mi a probléma?

A jelenlegi HydrAIDE chronicler **sok kis fájlt** hoz létre swamp-onként (akár 100-1000 db), ami:
- **ZFS metaadat overhead**: ~1KB/fájl → millió fájlnál ~100GB csak metaadat
- **4KB blokk alignment**: 500 byte adat → 4KB foglalás (8× overhead)
- **Lassú mentés/backup**: millió kis fájl másolása órákig tart
- **SSD wear**: sok random write kiírja az SSD-t
- **Módosítás költsége**: egy adat változásához az egész chunk-ot újra kell írni

### Mi a megoldás?

**Append-Only Block Storage**: Minden swamp **egyetlen fájl**, amelybe csak hozzáfűzünk (append), és időszakos compaction-nel takarítunk.

### Fő jellemzők:
- **1 fájl per swamp** (folder helyett)
- **Append-only írás** (nincs random I/O)
- **16KB blokkok** (ZFS-hez optimális)
- **Snappy tömörítés** blokkonként
- **Automatikus compaction** mentéskor, ha szükséges
- **Teljes visszafelé kompatibilitás** migrációval

---

## 2. Jelenlegi vs. Új Rendszer

### 2.1 Fájlstruktúra

```
JELENLEGI:                              ÚJ:
data/                                   data/
├── words/                              ├── words/
│   ├── ap/                             │   ├── ap/
│   │   └── apple/        ← FOLDER      │   │   └── apple.hyd    ← FÁJL!
│   │       ├── a1b2c3.dat              │   ├── ba/
│   │       ├── d4e5f6.dat              │   │   └── banana.hyd   ← FÁJL!
│   │       ├── g7h8i9.dat              │   └── ch/
│   │       └── meta.json               │       └── cherry.hyd   ← FÁJL!
│   ├── ba/
│   │   └── banana/       ← FOLDER
│   │       ├── j1k2l3.dat
│   │       └── meta.json
│   └── ...

Fájlszám (1M szó):                      Fájlszám (1M szó):
~100M fájl                              ~1M fájl (100× kevesebb!)
```

### 2.2 Fájl Belső Struktúra

```
┌─────────────────────────────────────────────────────────────────────────┐
│ FILE HEADER (64 byte)                                                   │
│ ├─ magic:         [4]byte  = "HYDR"                                     │
│ ├─ version:       uint16   = 2                                          │
│ ├─ flags:         uint16   (reserved)                                   │
│ ├─ created_at:    int64    (unix nano)                                  │
│ ├─ block_size:    uint32   = 16384 (16KB max)                           │
│ ├─ entry_count:   uint64   (összes live entry - compaction után frissül)│
│ └─ reserved:      [32]byte                                              │
├─────────────────────────────────────────────────────────────────────────┤
│ BLOCK 0                                                                 │
│ ├─ Block Header (16 byte):                                              │
│ │   ├─ compressed_size:   uint32                                        │
│ │   ├─ uncompressed_size: uint32                                        │
│ │   ├─ entry_count:       uint16                                        │
│ │   ├─ checksum:          uint32 (CRC32)                                │
│ │   └─ flags:             uint16                                        │
│ └─ Compressed Data (Snappy):                                            │
│     ├─ Entry 0: [op:1][key_len:2][key:N][data_len:4][data:M]            │
│     ├─ Entry 1: [op:1][key_len:2][key:N][data_len:4][data:M]            │
│     └─ ...                                                              │
├─────────────────────────────────────────────────────────────────────────┤
│ BLOCK 1                                                                 │
│ └─ ...                                                                  │
├─────────────────────────────────────────────────────────────────────────┤
│ BLOCK N (utolsó - lehet részlegesen teli)                               │
│ └─ ...                                                                  │
└─────────────────────────────────────────────────────────────────────────┘

Entry Operation Types:
- 0x01 = INSERT (új adat)
- 0x02 = UPDATE (módosítás - ugyanaz mint INSERT, de szemantikailag más)
- 0x03 = DELETE (törlés - csak key, nincs data)
```

### 2.3 Működési Elv

#### LOAD (Swamp megnyitása)

```
1. Fájl megnyitása
2. Header olvasása és validálása
3. Blokkok végigolvasása sorrendben:
   for each block:
       - Block header olvasása
       - Compressed data olvasása
       - Snappy decompress
       - Entry-k feldolgozása:
         for each entry:
             if entry.op == DELETE:
                 beaconKey.Delete(entry.key)
             else:
                 treasure = Treasure.FromBytes(entry.data)
                 beaconKey.Add(treasure)  // felülírja ha létezik
                 
4. A beaconKey-ben csak a "live" adatok maradnak
5. Fragmentation számítás: (összes entry - live entry) / összes entry
```

#### SAVE (Mentés)

```
1. Treasures összegyűjtése a treasuresWaitingForWriter-ből
2. Entry-k létrehozása:
   for each treasure:
       if treasure.IsDeleted():
           entry = {op: DELETE, key: treasure.key}
       else:
           entry = {op: UPDATE, key: treasure.key, data: treasure.ToBytes()}
           
3. Buffer-be gyűjtés és flush:
   for each entry:
       buffer.Add(entry)
       if buffer.size >= 16KB:
           block = Snappy.Compress(buffer)
           file.Append(block)
           buffer.Clear()
   
   // Maradék flush (force)
   if buffer.size > 0:
       block = Snappy.Compress(buffer)
       file.Append(block)

4. Fragmentation ellenőrzés:
   if fragmentation > 50%:
       Compact()
```

#### COMPACT (Töredezettség-mentesítés)

```
1. Observer védelem bekapcsolása (shutdown tiltás)
2. Új fájl létrehozása: swamp.hyd.new
3. Csak live adatok másolása:
   for each treasure in beaconKey:
       newFile.AppendEntry(treasure)
       
4. Atomic swap:
   rename(swamp.hyd.new, swamp.hyd)
   
5. Observer védelem kikapcsolása
```

---

## 3. Teljesítmény Összehasonlítás

### 3.1 I/O Műveletek

| Művelet | Jelenlegi | Új Rendszer | Javulás |
|---------|-----------|-------------|---------|
| **1 kulcs írása** | Read chunk (~250KB) + Write chunk | Append ~1KB | **~250×** |
| **100 kulcs írása** | 100× (Read + Write) | 1× Append ~16KB | **~1500×** |
| **1 kulcs olvasása** | Memóriából | Memóriából | **Ugyanaz** |
| **Swamp megnyitása** | Read all chunks | Read single file | **~2× gyorsabb** |

### 3.2 Lemezterület

| Metrika | Jelenlegi | Új Rendszer | Javulás |
|---------|-----------|-------------|---------|
| **Fájlok száma (1M swamp)** | ~100M | ~1M | **100×** |
| **ZFS metaadat** | ~100GB | ~1GB | **100×** |
| **4KB alignment waste** | ~8× overhead | ~1.2× overhead | **~7×** |
| **Tényleges helyfoglalás** | Magas | Compaction között nő | - |

### 3.3 Sebesség Mérések (Becsült, Samsung 990 PRO)

| Művelet | Jelenlegi | Új Rendszer |
|---------|-----------|-------------|
| **Single write latency** | ~0.5ms | ~0.02ms |
| **Batch write (100 keys)** | ~50ms | ~0.5ms |
| **Swamp open (25MB)** | ~20ms | ~15ms |
| **Compaction (25MB → 10MB)** | N/A | ~30ms |

### 3.4 SSD Élettartam

| Metrika | Jelenlegi | Új Rendszer |
|---------|-----------|-------------|
| **Napi írás (1M szó, 10 save)** | ~4TB/nap | ~40GB/nap |
| **990 PRO élettartam (1200TBW)** | ~300 nap | ~30000 nap (~82 év) |

---

## 4. Visszafelé Kompatibilitás

### 4.1 Verzió Kezelés (Migráció után)

Mivel a migrációt **standalone tool-lal, egyszerre** végezzük el, a chronicler kód egyszerűsödik:

```go
func (c *chronicler) Load() {
    // Migráció után CSAK V2 fájlok léteznek!
    // A V1 kód teljesen eltávolítható a kódbázisból.
    
    if !fileExists(c.hydFilePath) {
        // Új swamp, nincs mit betölteni
        return
    }
    
    // V2 betöltés
    c.loadV2()
}
```

**Előnyök:**
- Nincs verzió detektálás overhead
- Nincs V1 legacy kód karbantartás
- Tiszta, egyszerű kódbázis
- Nincs race condition kockázat

### 4.2 Migráció: Standalone Egyszeri Átalakítás

> **FONTOS:** A migrációt NEM menet közben csináljuk, hanem egy **önálló, egyszeri folyamatként**, 
> tervezett leállás alatt (pl. hétvégén). Ez sokkal biztonságosabb és egyszerűbb!

#### Miért jobb az egyszeri migráció?

| Menet közbeni migráció | Egyszeri standalone migráció |
|------------------------|------------------------------|
| ❌ Bonyolult verzió detektálás | ✅ Egyszerű: minden V1 → V2 |
| ❌ Race condition kockázat | ✅ Nincs versenyhelyzet |
| ❌ Kétféle kód karbantartása | ✅ Migráció után V1 kód törölhető |
| ❌ Teljesítmény overhead | ✅ Optimális futás |
| ❌ Rollback bonyolult | ✅ Backup → egyszerű rollback |

#### Migráció Folyamat

```
1. ELŐKÉSZÍTÉS
   ├── Trendizz szolgáltatás leállítása
   ├── Teljes backup készítése (ZFS snapshot vagy rsync)
   └── Migrátor tool indítása

2. FÁJLBEJÁRÁS (rekurzív)
   data/
   ├── words/
   │   ├── ap/
   │   │   └── apple/          ← FOLDER detektálva
   │   │       ├── uuid1.dat
   │   │       ├── uuid2.dat
   │   │       └── meta.json
   │   ...
   
3. SWAMP MIGRÁCIÓ (egyesével)
   for each swamp_folder:
       a) V1 Load: beolvassa az összes chunk-ot és meta.json-t
       b) V2 Write: egyetlen .hyd fájlba ír (append-only blokkok)
       c) Verify: visszaolvasás és összehasonlítás
       d) Cleanup: régi folder törlése
       e) Log: migráció státusz

4. BEFEJEZÉS
   ├── Migráció log ellenőrzése (hibák?)
   ├── Statisztika: hány swamp, mennyi idő, helyfoglalás előtte/utána
   └── Trendizz szolgáltatás indítása (már V2 kóddal!)
```

#### Migrátor Tool Interface

```go
// Standalone CLI tool: hydraidectl migrate
type MigrationConfig struct {
    DataPath        string        // pl. "/var/hydraide/data"
    DryRun          bool          // ELSŐ FUTÁS: csak validál, nem ír semmit
    Parallel        int           // párhuzamos worker-ek száma
    VerifyAfter     bool          // minden swamp után verify (csak live módban)
    DeleteOldFiles  bool          // régi fájlok törlése (csak live módban)
    StopOnError     bool          // első hibánál megáll, vagy folytatja
    ProgressReport  time.Duration // progress log gyakoriság
}

type MigrationResult struct {
    TotalSwamps      int
    ProcessedSwamps  int
    SuccessfulSwamps int
    FailedSwamps     []FailedSwamp
    TotalEntries     int64
    OldSizeBytes     int64
    NewSizeBytes     int64      // dry-run esetén becsült méret
    Duration         time.Duration
    DryRun           bool
}

type FailedSwamp struct {
    Path   string
    Error  string
    Phase  string  // "load", "convert", "write", "verify"
}
```

#### Kétlépcsős Migráció

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. LÉPÉS: DRY-RUN (pénteken, működő rendszer mellett is futhat)             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  $ hydraidectl migrate --data-path=/var/hydraide/data --dry-run             │
│                                                                             │
│  Mit csinál:                                                                │
│  ├── Végigmegy minden swamp folder-en                                       │
│  ├── Beolvassa a V1 adatokat (CSAK OLVASÁS!)                                │
│  ├── Ellenőrzi, hogy minden adat deszerializálható-e                        │
│  ├── Ellenőrzi, hogy a konverzió hibátlan lenne-e                           │
│  ├── Kiszámítja a becsült új méretet                                        │
│  ├── NEM ír semmit a lemezre!                                               │
│  └── Riportot generál: hibák listája, statisztikák                          │
│                                                                             │
│  Output:                                                                    │
│  ├── migration-dryrun-2026-01-21.log   (részletes log)                      │
│  └── migration-dryrun-2026-01-21.json  (gépi feldolgozáshoz)                │
│                                                                             │
│  Ha HIBA van:                                                               │
│  ├── Hibás swamp-ok listája                                                 │
│  ├── Hiba típusa és fázisa                                                  │
│  └── Javítás ELŐTTE, majd újra dry-run                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ Ha minden OK
┌─────────────────────────────────────────────────────────────────────────────┐
│ 2. LÉPÉS: LIVE MIGRÁCIÓ (hétvégén, leállított rendszer mellett)             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  # Előtte: Trendizz leállítása + ZFS snapshot / backup                      │
│                                                                             │
│  $ hydraidectl migrate --data-path=/var/hydraide/data \                     │
│                        --verify \                                           │
│                        --delete-old \                                       │
│                        --parallel=8                                         │
│                                                                             │
│  Mit csinál:                                                                │
│  ├── Végigmegy minden swamp folder-en                                       │
│  ├── Beolvassa a V1 adatokat                                                │
│  ├── Konvertálja V2 formátumra                                              │
│  ├── Kiírja az új .hyd fájlt                                                │
│  ├── Verify: visszaolvassa és összehasonlítja                               │
│  ├── Törli a régi folder-t                                                  │
│  └── Riportot generál                                                       │
│                                                                             │
│  # Utána: Trendizz indítása az új V2 kóddal                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Dry-Run Részletes Működés

```go
func (m *Migrator) DryRun() MigrationResult {
    result := MigrationResult{DryRun: true}
    
    swampFolders := m.walkAndFindSwampFolders()
    result.TotalSwamps = len(swampFolders)
    
    for _, folder := range swampFolders {
        result.ProcessedSwamps++
        
        // 1. Tudunk-e olvasni?
        treasures, meta, err := m.tryLoadV1(folder)
        if err != nil {
            result.FailedSwamps = append(result.FailedSwamps, FailedSwamp{
                Path:  folder,
                Error: err.Error(),
                Phase: "load",
            })
            if m.config.StopOnError {
                break
            }
            continue
        }
        
        // 2. Minden treasure deszerializálható?
        for _, t := range treasures {
            if err := t.Validate(); err != nil {
                result.FailedSwamps = append(result.FailedSwamps, FailedSwamp{
                    Path:  folder,
                    Error: fmt.Sprintf("treasure %s: %v", t.GetKey(), err),
                    Phase: "convert",
                })
                break
            }
        }
        
        // 3. Méret kalkuláció (becsült)
        result.OldSizeBytes += getFolderSize(folder)
        result.NewSizeBytes += estimateV2Size(treasures)
        result.TotalEntries += int64(len(treasures))
        
        result.SuccessfulSwamps++
        m.logProgress(result)
    }
    
    // 4. Riport generálás
    m.generateReport(result)
    
    return result
}
```

#### CLI Használat Példák

```bash
# 1. Dry-run: minden swamp validálása
hydraidectl migrate --data-path=/var/hydraide/data --dry-run

# 2. Dry-run: részletes output + nem áll meg hibánál
hydraidectl migrate --data-path=/var/hydraide/data --dry-run --continue-on-error

# 3. Live migráció: verify + régi fájlok törlése + 8 worker
hydraidectl migrate --data-path=/var/hydraide/data --verify --delete-old --parallel=8

# 4. Live migráció: csak egy almappa (teszteléshez)
hydraidectl migrate --data-path=/var/hydraide/data/words/ap --verify --delete-old
```

#### Dry-Run Output Példa

```
================================================================================
HydrAIDE Migration Dry-Run Report
Date: 2026-01-21 14:30:00
================================================================================

SUMMARY:
  Total swamps found:     1,234,567
  Successfully validated: 1,234,560
  Failed:                 7
  
SIZE ESTIMATION:
  Current size (V1):      847.3 GB
  Estimated size (V2):    312.5 GB
  Estimated savings:      534.8 GB (63.1%)

ENTRIES:
  Total treasures:        45,678,901
  Average per swamp:      37

FAILED SWAMPS:
  1. /var/hydraide/data/words/co/corrupted-word/
     Phase: load
     Error: invalid meta.json: unexpected EOF
     
  2. /var/hydraide/data/words/te/test-bad/
     Phase: convert  
     Error: treasure "key123": binary data corrupted (checksum mismatch)
     
  ... (5 more)

RECOMMENDATION:
  ❌ 7 swamps need manual inspection before live migration.
  Fix the issues and re-run dry-run.
  
================================================================================
```

#### Live Migráció Pszeudokód

```go
func (m *Migrator) LiveMigrate() MigrationResult {
    result := MigrationResult{DryRun: false}
    
    // 1. Fájlrendszer bejárása
    swampFolders := m.walkAndFindSwampFolders()
    result.TotalSwamps = len(swampFolders)
    
    // 2. Párhuzamos migráció (worker pool)
    workerPool := NewWorkerPool(m.config.Parallel)
    
    for _, folder := range swampFolders {
        workerPool.Submit(func() error {
            // 2a. V1 Load
            treasures, meta, err := m.loadV1Swamp(folder)
            if err != nil {
                return fmt.Errorf("load failed: %w", err)
            }
            result.OldSizeBytes += getFolderSize(folder)
            
            // 2b. V2 Write
            hydFile := folder + ".hyd"  // apple/ → apple.hyd
            if err := m.writeV2File(hydFile, treasures, meta); err != nil {
                return fmt.Errorf("write failed: %w", err)
            }
            result.NewSizeBytes += getFileSize(hydFile)
            
            // 2c. Verify
            if m.config.VerifyAfter {
                if err := m.verifyMigration(folder, hydFile, treasures); err != nil {
                    // Hiba esetén töröljük az új fájlt, régi marad
                    os.Remove(hydFile)
                    return fmt.Errorf("verify failed: %w", err)
                }
            }
            
            // 2d. Cleanup - CSAK sikeres verify után!
            if m.config.DeleteOldFiles {
                os.RemoveAll(folder)
            }
            
            result.SuccessfulSwamps++
            result.TotalEntries += int64(len(treasures))
            return nil
        })
    }
    
    // 3. Várakozás és eredmény
    workerPool.Wait()
    m.generateReport(result)
    return result
}

func (m *Migrator) verifyMigration(oldFolder, newFile string, originalTreasures []treasure.Treasure) error {
    // Visszaolvassuk az új fájlt
    loadedTreasures, err := m.loadV2File(newFile)
    if err != nil {
        return fmt.Errorf("cannot load new file: %w", err)
    }
    
    // Darabszám egyezés
    if len(loadedTreasures) != len(originalTreasures) {
        return fmt.Errorf("count mismatch: expected %d, got %d", 
            len(originalTreasures), len(loadedTreasures))
    }
    
    // Minden treasure összehasonlítása
    for _, orig := range originalTreasures {
        loaded, exists := loadedTreasures[orig.GetKey()]
        if !exists {
            return fmt.Errorf("missing key: %s", orig.GetKey())
        }
        
        if !orig.Equals(loaded) {
            return fmt.Errorf("content mismatch for key: %s", orig.GetKey())
        }
    }
    
    return nil
}
```

#### Becsült Migráció Idő

| Swamp szám | Becsült idő (8 worker) | Megjegyzés |
|------------|------------------------|------------|
| 10,000 | ~2-3 perc | Kis adatbázis |
| 100,000 | ~20-30 perc | Közepes |
| 1,000,000 | ~3-4 óra | Nagy (Trendizz) |
| 10,000,000 | ~30-40 óra | Nagyon nagy |

*Megjegyzés: 990 PRO SSD-vel, verify-val együtt*

#### Rollback Terv

```
HA BÁRMI HIBA:
1. Migrátor leáll az első hibánál (vagy folytatja, konfig függő)
2. Hibalista generálása
3. ZFS snapshot restore VAGY backup visszaállítás
4. Hibajavítás
5. Újrapróbálás
```

---

## 5. Implementációs Fázisok

### 5.1 Fázis 1: Alap Infrastruktúra ✅ KÉSZ

- [x] Új `chronicler/v2/` package létrehozása
- [x] File header struktúra és serialization (`types.go`)
- [x] Block header struktúra és serialization (`types.go`)
- [x] Entry struktúra és serialization (`types.go`)
- [x] Snappy compression integráció (saját `compressor` csomag használata)

**Fázis státusz:** ✅ Kész

### 5.2 Fázis 2: Írási Műveletek ✅ KÉSZ

- [x] Write buffer implementáció (`block.go`)
- [x] Block flush logika (`block.go`)
- [x] Append-only file writer (`writer.go`)
- [x] Force flush (shutdown/explicit save)

**Fázis státusz:** ✅ Kész

### 5.3 Fázis 3: Olvasási Műveletek ✅ KÉSZ

- [x] File reader és block parser (`reader.go`)
- [x] Entry processor (INSERT/UPDATE/DELETE)
- [x] Fragmentation tracking (`CalculateFragmentation`)
- [x] Index rebuild optimalizáció (`LoadIndex`)

**Fázis státusz:** ✅ Kész

### 5.4 Fázis 4: Compaction ✅ KÉSZ

- [x] Compaction algoritmus (`compactor.go`)
- [x] Observer integráció előkészítés (shutdown védelem placeholder)
- [x] Atomic file swap
- [x] Fragmentation threshold konfiguráció

**Fázis státusz:** ✅ Kész

### 5.5 Fázis 5: Standalone Migrátor Tool

- [ ] `hydraidectl migrate` command implementáció
- [ ] Rekurzív folder bejárás (V1 swamp detektálás)
- [ ] V1 → V2 konverter (load V1, write V2)
- [ ] Verify logika (visszaolvasás és összehasonlítás)
- [ ] Párhuzamos worker pool (konfigurálható)
- [ ] Progress reporting és logging
- [ ] Dry-run mód (szimuláció)
- [ ] Cleanup (régi fájlok törlése)
- [ ] Rollback támogatás (hiba esetén)
- [ ] Statisztika generálás (méret előtte/utána, idő, hibák)

**Becsült idő:** 3-4 nap

### 5.6 Fázis 6: Integráció

- [ ] Chronicler interface bővítése
- [ ] Swamp integráció (chronicler csere)
- [ ] Path kezelés módosítása (folder → file)
- [ ] Name package kompatibilitás

**Becsült idő:** 2-3 nap

### 5.7 Fázis 7: Migrációs Tesztelés (End-to-End)

> **KRITIKUS FÁZIS!** Teljes körű tesztelési eljárás a migráció validálására.

#### Tesztelési Stratégia:

```
1. RÉGI RENDSZERREL ADATOK LÉTREHOZÁSA
   ├── Teszt swamp-ok generálása (V1 formátum)
   ├── Különböző méretű swamp-ok (kicsi, közepes, nagy)
   ├── Edge case-ek: üres swamp, 1 entry, 100K+ entry
   └── Flush to disk → V1 fájlok lemezen

2. MIGRÁCIÓ VÉGREHAJTÁSA
   ├── Swamp megnyitása (Load detektálja V1-et)
   ├── Automatikus konverzió V2 formátumra
   ├── Régi fájlok törlése
   └── Új .hyd fájl létrehozása

3. VISSZAOLVASÁS ÉS VALIDÁCIÓ
   ├── Minden kulcs visszaolvasható?
   ├── Adatok byte-ra megegyeznek?
   ├── Metaadatok megmaradtak? (created_at, updated_at, stb.)
   └── Beacon indexek helyesen épülnek fel?

4. MŰKÖDÉSI TESZTEK MIGRÁCIÓT KÖVETŐEN
   ├── Új adatok írása (append működik?)
   ├── Meglévő adatok módosítása
   ├── Törlés (DELETE entry)
   ├── Compaction működik?
   └── Újraindítás után is minden OK?
```

#### Tesztelési Checklist:

- [ ] **V1 teszt fixture-ök generálása**
    - [ ] Kis swamp: 10 treasure, különböző típusokkal
    - [ ] Közepes swamp: 1000 treasure
    - [ ] Nagy swamp: 50000 treasure
    - [ ] Edge case: üres swamp (csak meta.json)
    - [ ] Edge case: 1 treasure
    - [ ] Edge case: törölt treasure-ök (shadowDelete)
    
- [ ] **Migráció tesztek**
    - [ ] V1 → V2 konverzió hibátlan
    - [ ] Régi fájlok törlődnek
    - [ ] Új .hyd fájl létrejön
    - [ ] Ha migráció közben crash → újra tud próbálkozni
    
- [ ] **Adatintegritás validáció**
    - [ ] Minden kulcs megtalálható migráció után
    - [ ] GetTreasure() ugyanazt adja vissza
    - [ ] Binary content byte-ra egyezik
    - [ ] Metaadatok: createdAt, modifiedAt, expirationTime
    - [ ] Shadow deleted treasure-ök megmaradnak
    
- [ ] **Működési tesztek V2-n**
    - [ ] SaveTreasure() új adat → append működik
    - [ ] SaveTreasure() módosítás → UPDATE entry
    - [ ] DeleteTreasure() → DELETE entry
    - [ ] GetTreasuresByKeys() batch read
    - [ ] GetAll() minden adat visszajön
    - [ ] Beacon lekérdezések (CreationTime, ExpirationTime, stb.)
    
- [ ] **Compaction tesztek**
    - [ ] 50%+ fragmentation → compact fut
    - [ ] Compact után minden adat megvan
    - [ ] Fájlméret csökken
    - [ ] Observer védelem működik (shutdown block)
    
- [ ] **Újraindítás tesztek**
    - [ ] Swamp close → reopen → minden adat megvan
    - [ ] Crash szimuláció (kill) → recovery
    - [ ] Félig írt block → skip/recover

- [ ] **Performance benchmark**
    - [ ] V1 vs V2 write speed összehasonlítás
    - [ ] V1 vs V2 load speed összehasonlítás
    - [ ] Compaction idő mérése
    - [ ] Memory footprint összehasonlítás

**Becsült idő:** 4-5 nap

### 5.8 Fázis 8: Dokumentáció

- [ ] API dokumentáció
- [ ] Migráció útmutató
- [ ] Performance riport
- [ ] CHANGELOG frissítés

**Becsült idő:** 1 nap

---

## 6. Konfigurációs Paraméterek

```go
type ChroniclerV2Config struct {
    // Block méret (alapértelmezett: 16KB, ZFS-hez optimális)
    MaxBlockSize int  // default: 16384
    
    // Compaction küszöb (alapértelmezett: 50%)
    CompactionThreshold float64  // default: 0.5
    
    // Force compaction swamp close-kor
    CompactOnClose bool  // default: true
    
    // Flush timeout (ha nincs elég adat a blokkhoz)
    FlushTimeout time.Duration  // default: 100ms
}
```

---

## 7. Kockázatok és Megoldások

| Kockázat | Valószínűség | Hatás | Megoldás |
|----------|--------------|-------|----------|
| **Adatvesztés migrációkor** | Alacsony | Magas | Backup készítés előtte, kétlépcsős migráció |
| **Compaction közben crash** | Közepes | Közepes | Atomic rename, régi fájl megtartása amíg az új kész |
| **Memory spike nagy swamp-nál** | Közepes | Alacsony | Streaming load, nem egyszerre az egész fájl |
| **Lassabb startup** | Alacsony | Alacsony | Sequential read gyors, elhanyagolható különbség |

---

## 8. Összefoglalás

### Amit nyerünk:
- ✅ **100× kevesebb fájl** a lemezen
- ✅ **~100GB megtakarítás** ZFS metaadat overhead-en
- ✅ **~10-50× gyorsabb írás** (append vs. rewrite)
- ✅ **~100× hosszabb SSD élettartam**
- ✅ **Sokkal gyorsabb backup/mentés**
- ✅ **Egyszerűbb fájlstruktúra**

### Amit veszítünk:
- ⚠️ **Időszakos compaction szükséges** (automatikus)
- ⚠️ **Migráció szükséges** (egyszer)
- ⚠️ **Kicsit bonyolultabb chronicler** (de tisztább!)

### Becsült össz implementációs idő:
**20-26 munkanap**

---

## 9. Jóváhagyás

| Név | Szerep | Dátum | Státusz |
|-----|--------|-------|---------|
| Péter | Architect | | ⏳ Függőben |

---

**Következő lépés:** Péter jóváhagyása után a Fázis 1 megkezdése.

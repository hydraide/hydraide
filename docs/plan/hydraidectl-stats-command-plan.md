# HydrAIDECtl Stats Command - Kivitelezési Terv

## Összefoglaló

**Cél**: Új `hydraidectl stats --instance <name>` parancs létrehozása, amely részletes statisztikákat és "egészségi jelentést" ad a HydrAIDE V2 swamp-okról.

**Érintett komponensek**:
- `app/hydraidectl/cmd/stats.go` (új fájl)
- `app/hydraidectl/cmd/root.go` (dokumentáció frissítés)
- `docs/hydraidectl/hydraidectl-user-manual.md` (dokumentáció)

---

## Meglévő állapot elemzése

### Újrafelhasználható komponensek:
- ✅ `v2.FileReader` és `CalculateFragmentation()` - töredezettség számítás
- ✅ `buildmeta.GetInstance()` - instance metadata lekérés  
- ✅ `schollz/progressbar/v3` - progress bar (már van a projektben)
- ✅ `size.go` - swamp bejárási logika és struktúra
- ✅ `migrate.go` - worker pool pattern, V2 fájl detektálás

### Új funkciók:
- ❌ Részletes töredezettségi statisztika gyűjtés
- ❌ Jelentés mentése és visszaolvasása
- ❌ ETA számítás a loaderhez
- ❌ Szép táblázatos megjelenítés (tablewriter csomag)

---

## Statisztikák listája

A következő adatokat fogjuk összegyűjteni és megjeleníteni:

### Összefoglaló statisztikák
| Statisztika | Leírás |
|-------------|--------|
| Total Database Size | Teljes adatbázis méret (MB/GB) |
| Total Swamps | Swamp-ok száma összesen |
| Total Records | Összes élő rekord szám |
| Total Entries | Összes entry (beleértve a töröltet is) |
| Average Records per Swamp | Átlagos rekordszám swamp-onként |
| Median Records per Swamp | Medián rekordszám |
| Average Swamp Size | Átlagos swamp méret |
| Oldest Swamp | Legrégebbi swamp (módosítási idő) |
| Newest Swamp | Legújabb swamp (módosítási idő) |

### Top 10 Legnagyobb swamp-ok
- Swamp neve
- Méret (MB)
- Record szám

### Top 10 Legtöredezettebb swamp-ok
- Swamp neve  
- Töredezettség (%)
- Dead entries (darab)
- Live entries (darab)
- Compaction ajánlás (igen/nem)

### Compaction összefoglaló
| Statisztika | Leírás |
|-------------|--------|
| Swamps needing compaction | >20% töredezettségű swamp-ok száma |
| Total reclaimable space | Becsült felszabadítható hely |
| Average fragmentation | Átlagos töredezettség (%) |

### Scan információ
- Scan duration (mennyi idő volt a scan)
- Timestamp (mikor készült)

---

## Fázisok

### 1. Fázis: Alapstruktúra és analyzer engine

- [x] `stats.go` fájl létrehozása cobra command-dal
- [x] Flags definiálása: `--instance`, `--json`, `--latest`, `--parallel`
- [x] `SwampStats` és `StatsReport` struktúrák definiálása
- [x] `SwampAnalyzer` létrehozása a V2 fájlok elemzéséhez
- [x] V2 swamp-ok keresése (`.hyd` fájlok)
- [x] Worker pool implementálása (4 worker default)

**Fázis státusz:** ✅ Kész

---

### 2. Fázis: Adatgyűjtés és töredezettség számítás

- [x] Swamp fájl méret lekérés
- [x] `v2.FileReader` használata minden swamp-hoz
- [x] `CalculateFragmentation()` hívása minden swamp-ra
- [x] Header információk kinyerése (EntryCount, CreatedAt, ModifiedAt)
- [x] Live/dead entry számítás
- [x] Összesítő statisztikák számítása (átlag, medián, top 10-ek)

**Fázis státusz:** ✅ Kész

---

### 3. Fázis: Progress bar és ETA

- [x] `schollz/progressbar/v3` integrálása
- [x] Swamp számláló megjelenítése ("Scanning X/Y swamps...")
- [x] ETA számítás (átlagos swamp feldolgozási idő alapján)
- [x] Spinner/progress megjelenítés a scan alatt

**Fázis státusz:** ✅ Kész

---

### 4. Fázis: Jelentés mentése és `--latest` flag

- [x] Working folder meghatározása: `<instance_base_path>/.hydraide/`
- [x] `stats-report-latest.json` fájl mentése a working folderbe
- [x] `--latest` flag implementálása - legutóbbi jelentés visszaolvasása
- [x] Timestamp és verzió mentése a jelentésbe

**Fázis státusz:** ✅ Kész

---

### 5. Fázis: Szép táblázatos megjelenítés

- [x] Custom táblázat formázás (tablewriter helyett, mert v1.1.3 API változott)
- [x] Összefoglaló táblázat formázása
- [x] Top 10 legnagyobb swamp táblázat
- [x] Top 10 legtöredezettebb swamp táblázat
- [x] Compaction ajánlások megjelenítése
- [x] Színezés/emoji-k hozzáadása (⚠️ warning, ✅ ok, stb.)

**Fázis státusz:** ✅ Kész

---

### 6. Fázis: JSON output

- [x] `--json` flag implementálása
- [x] Teljes `StatsReport` JSON serializálása
- [x] Pretty-print és compact mód

**Fázis státusz:** ✅ Kész

---

### 7. Fázis: Dokumentáció

- [x] `hydraidectl-user-manual.md` frissítése a `stats` paranccsal
- [x] Példa output-ok dokumentálása
- [x] `root.go` Long description frissítése

**Fázis státusz:** ✅ Kész

---

### 8. Fázis: Commit, Push, Tag

- [ ] Kód átnézés és tesztelés
- [ ] Commit message: `feat(hydraidectl): add stats command for swamp analysis`
- [ ] Push to remote
- [ ] Tag létrehozása: `hydraidectl/v2.1.3`

**Fázis státusz:** ⏳ Folyamatban

---

## Technikai részletek

### Struktúra definíciók

```go
type SwampStats struct {
    Path             string    `json:"path"`
    Name             string    `json:"name"`
    SizeBytes        int64     `json:"size_bytes"`
    LiveEntries      int       `json:"live_entries"`
    TotalEntries     int       `json:"total_entries"`
    DeadEntries      int       `json:"dead_entries"`
    Fragmentation    float64   `json:"fragmentation_percent"`
    NeedsCompaction  bool      `json:"needs_compaction"`
    CreatedAt        time.Time `json:"created_at"`
    ModifiedAt       time.Time `json:"modified_at"`
}

type StatsReport struct {
    Instance         string       `json:"instance"`
    GeneratedAt      time.Time    `json:"generated_at"`
    ScanDuration     string       `json:"scan_duration"`
    
    // Summary
    TotalDatabaseSize    int64   `json:"total_database_size_bytes"`
    TotalSwamps          int     `json:"total_swamps"`
    TotalLiveRecords     int64   `json:"total_live_records"`
    TotalEntries         int64   `json:"total_entries"`
    TotalDeadEntries     int64   `json:"total_dead_entries"`
    AvgRecordsPerSwamp   float64 `json:"avg_records_per_swamp"`
    MedianRecordsPerSwamp int    `json:"median_records_per_swamp"`
    AvgSwampSize         int64   `json:"avg_swamp_size_bytes"`
    
    // Fragmentation
    AvgFragmentation         float64 `json:"avg_fragmentation_percent"`
    SwampsNeedingCompaction  int     `json:"swamps_needing_compaction"`
    ReclaimableSpace         int64   `json:"reclaimable_space_bytes"`
    
    // Dates
    OldestSwamp    *SwampStats  `json:"oldest_swamp,omitempty"`
    NewestSwamp    *SwampStats  `json:"newest_swamp,omitempty"`
    
    // Top lists
    LargestSwamps           []SwampStats `json:"largest_swamps"`
    MostFragmentedSwamps    []SwampStats `json:"most_fragmented_swamps"`
    
    // All swamps (for detailed analysis)
    AllSwamps    []SwampStats `json:"all_swamps,omitempty"`
}
```

### Working folder
- Path: `<instance_base_path>/.hydraide/`
- Stats report: `.hydraide/stats-report-latest.json`

### Compaction threshold
- Default: 20% töredezettség felett jelezzük, hogy compaction ajánlott

---

## Flags összefoglaló

| Flag | Rövid | Leírás | Default |
|------|-------|--------|---------|
| `--instance` | `-i` | Instance neve (kötelező) | - |
| `--json` | `-j` | JSON output | false |
| `--latest` | `-l` | Legutóbbi mentett jelentés megjelenítése | false |
| `--parallel` | `-p` | Worker szám | 4 |

---

## Jóváhagyásra vár

Péter, kérlek nézd át a tervet és jelezd, ha:
1. Módosítani szeretnél valamit
2. Bármit hozzá szeretnél adni
3. Jóváhagyod és kezdhetem a kivitelezést

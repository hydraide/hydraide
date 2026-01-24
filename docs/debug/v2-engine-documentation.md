# HydrAIDE V2 Engine - Működési Dokumentáció

**Verzió**: v2.1.5  
**Dátum**: 2026-01-22

---

## 📋 Tartalomjegyzék

1. [V1 → V2 Migráció](#1-v1--v2-migráció)
2. [Automatikus Defragmentálás (Compactor)](#2-automatikus-defragmentálás-compactor)
3. [Manuális Defragmentálás (hydraidectl compact)](#3-manuális-defragmentálás-hydraidectl-compact)
4. [Törlési Logika](#4-törlési-logika)
5. [Kiíró Procedúra (V2 Writer)](#5-kiíró-procedúra-v2-writer)
6. [Blokkméret és ZFS ajánlások](#6-blokkméret-és-zfs-ajánlások)

---

## 1. V1 → V2 Migráció

### 1.1 Probléma háttere

A V1 engine a következőképpen működött:
- Minden swamp egy **mappa** volt, több chunk fájllal
- Új adatok **APPEND** módban kerültek a chunk fájlokba
- Ha egy chunk megtelt, új chunk jött létre
- **Probléma**: Ha egy kulcs módosult és új chunk-ba került, a régi verzió **MEGMARADT** a régi chunk-ban

A V1 `Load` metódus **map-be** töltötte az adatokat, így implicit deduplikáció történt:
```go
for fileName, byteTreasures := range contents {
    for _, byteTreasure := range byteTreasures {
        treasures[treasureInterface.GetKey()] = treasureInterface  // Utolsó nyer!
    }
}
```

### 1.2 Migrátor működése

**Parancs**: `hydraidectl migrate --instance <name> --full`

**Folyamat**:

1. **V1 swamp mappák keresése**
   - Végigmegy a data mappán
   - Azonosítja a V1 swampokat (meta fájl + UUID nevű chunk fájlok)

2. **Chunk fájlok beolvasása**
   - Minden chunk fájlt dekompresszál (Snappy)
   - Kinyeri a benne lévő treasure-öket (GOB formátum)

3. **Deduplikáció** ⚠️ **KRITIKUS LÉPÉS**
   - **Map-ba** gyűjti az összes entry-t kulcs alapján
   - Ha ugyanaz a kulcs többször előfordul → **az utolsó verzió marad**
   - Ez pontosan megegyezik a V1 Load viselkedésével

   ```go
   entryMap := make(map[string]v2.Entry)
   for _, entry := range fileEntries {
       entryMap[entry.Key] = entry  // Utolsó nyer!
   }
   ```

4. **Üres swampok kezelése**
   - Ha a deduplikáció után **0 entry** marad → **NEM jön létre V2 fájl**
   - A régi V1 fájlok törlődnek (ha `--delete-old` engedélyezve)
   - Statisztikában: `EmptySwampsSkipped` számláló

5. **V2 fájl írása**
   - Először a metadata entry (swamp név)
   - Utána az összes deduplikált entry
   - 16KB-os blokkokba tömörítve (Snappy)

6. **Verifikáció** (opcionális, `--verify` flag)
   - Visszaolvassa a V2 fájlt
   - Ellenőrzi, hogy minden kulcs megvan

7. **V1 fájlok törlése** (opcionális, `--delete-old` flag)

### 1.3 Migrátor jelentés

```
SUMMARY:
  Total swamps found:     11526
  Successfully processed: 11500
  Empty swamps skipped:   26
  Failed:                 0

ENTRIES:
  Raw entries (before dedup): 2500000
  Deduplicated entries:       2200000
  Duplicate keys removed:     300000
  
  ⚠️  Duplicates were found and deduplicated!
     This is normal - V1 kept old versions in separate chunk files.
```

---

## 2. Automatikus Defragmentálás (Compactor)

### 2.1 Működési elv

A V2 engine **append-only** architektúrájú:
- Új adat → új entry hozzáfűzése
- Módosítás → UPDATE entry hozzáfűzése (régi megmarad)
- Törlés → DELETE entry hozzáfűzése (régi megmarad)

Ez fragmentációhoz vezet. A `Compactor` komponens kezeli ezt.

### 2.2 Fragmentáció számítása

```go
fragmentation = (összes entry - élő kulcsok) / összes entry
```

Például:
- 100 entry a fájlban
- 80 egyedi, élő kulcs
- Fragmentáció = (100 - 80) / 100 = 20%

### 2.3 LoadIndex működése

A `LoadIndex` függvény végigolvassa a fájlt és felépíti az élő kulcsok indexét:

```go
func LoadIndex() (map[string][]byte, string, error) {
    index := make(map[string][]byte)
    
    for each entry in file {
        switch entry.Operation {
        case OpDelete:
            delete(index, entry.Key)  // Kulcs törlése az indexből!
        case OpInsert, OpUpdate:
            index[entry.Key] = entry.Data  // Utolsó verzió marad
        }
    }
    
    return index, swampName, nil
}
```

**KRITIKUS**: Ha egy kulcs utolsó művelete DELETE → **a kulcs NINCS az indexben** → **NEM kerül be a compactolt fájlba**!

### 2.4 Compaction folyamat

1. **Fragmentáció ellenőrzése**
   - Ha < threshold (alapértelmezett 50%) → nincs teendő

2. **Index betöltése**
   - `LoadIndex()` hívás → csak az élő kulcsok maradnak

3. **Üres fájl kezelése**
   - Ha az index üres (minden törölve) → **fájl törlése**
   ```go
   if len(index) == 0 {
       os.Remove(filePath)
       result.FileDeleted = true
       return
   }
   ```

4. **Új fájl írása**
   - Temp fájl létrehozása (`.compact` kiterjesztés)
   - Metadata entry írása (ha van swamp név)
   - Összes élő entry írása (INSERT műveletként)
   - Atomi csere: `os.Rename(tempFile, originalFile)`

---

## 3. Manuális Defragmentálás (hydraidectl compact)

### 3.1 Parancs

```bash
# Dry-run (csak elemzés)
hydraidectl compact --instance <name> --dry-run

# Tényleges compaction
hydraidectl compact --instance <name> --parallel 4

# Compaction + újraindítás
hydraidectl compact --instance <name> --parallel 4 --restart
```

### 3.2 Opciók

| Flag | Leírás | Alapértelmezett |
|------|--------|-----------------|
| `--instance` | Instance neve | (kötelező) |
| `--parallel` | Párhuzamos workerek száma | 4 |
| `--threshold` | Fragmentáció küszöb (%) | 20 |
| `--restart` | Újraindítás compaction után | false |
| `--dry-run` | Csak elemzés | false |
| `--json` | JSON kimenet | false |

### 3.3 Folyamat

1. **Instance leállítása** (ha fut)
2. **V2 swamp fájlok keresése** (`.hyd` kiterjesztés)
3. **Párhuzamos compaction** worker pool-lal
4. **Jelentés**:
   - Compactolt swampok száma
   - Törölt swampok (üres fájlok)
   - Megtakarított hely
   - Eltávolított entry-k

### 3.4 Kimenet példa

```
📊 SUMMARY
────────────────────────────────────────────────
Total Swamps             │ 11500
Scanned                  │ 11500
Compacted                │ 7602
Deleted (empty)          │ 🗑️  26
Skipped (below threshold)│ 3872
Duration                 │ 45s

💾 SPACE ANALYSIS
────────────────────────────────────────────────
Size Before              │ 261.41 MB
Size After               │ 180.23 MB
Space Saved              │ ✅ 81.18 MB
Savings                  │ 31.1%
Entries Removed          │ 312000
```

---

## 4. Törlési Logika

### 4.1 Soft Delete vs Hard Delete

**Soft Delete** (shadowDelete):
- A rekord törlésre jelölve, de **fizikailag megmarad**
- Lekérdezhető marad bizonyos feltételekkel

**Hard Delete**:
- DELETE entry íródik a fájlba
- A kulcs eltűnik az indexből
- Compaction során **fizikailag is törlődik**

### 4.2 Teljes fájl törlés

Egy swamp fájl **teljesen törlődik**, ha:
1. Minden kulcs DELETE művelettel zárul
2. `LoadIndex()` üres map-ot ad vissza
3. Compactor ezt észleli és törli a fájlt

### 4.3 Üres swamp kezelése migrációkor

Ha a V1 swamp:
- Csak üres chunk fájlokat tartalmaz, VAGY
- Minden treasure törölve volt

Akkor:
- **NEM jön létre V2 fájl**
- A V1 mappa törlődik (ha `--delete-old` aktív)
- Statisztikában: `EmptySwampsSkipped++`

---

## 5. Kiíró Procedúra (V2 Writer)

### 5.1 FileWriter működése

```go
writer := NewFileWriter(filePath, maxBlockSize)

// Entry hozzáadása
writer.WriteEntry(entry)  // Bufferbe kerül

// Ha buffer >= maxBlockSize → automatikus flush
// Vagy manuális:
writer.Flush()

// Lezárás (flush + header update + sync)
writer.Close()
```

### 5.2 WriteBuffer

A `WriteBuffer` gyűjti az entry-ket, amíg el nem éri a `maxBlockSize`-t:

```go
type WriteBuffer struct {
    entries     []Entry
    currentSize int
    maxSize     int  // Alapértelmezett: 16KB
}

func (wb *WriteBuffer) Add(entry Entry) bool {
    wb.entries = append(wb.entries, entry)
    wb.currentSize += entry.Size()
    return wb.currentSize >= wb.maxSize  // Flush szükséges?
}
```

### 5.3 Blokk formátum

Amikor a buffer flushol:
1. Entry-k szerializálása (bináris formátum)
2. Snappy tömörítés
3. CRC32 checksum számítása
4. Block header írása (16 byte)
5. Tömörített adat írása

```
[Block Header: 16 bytes]
├── CompressedSize:   4 bytes (uint32)
├── UncompressedSize: 4 bytes (uint32)
├── EntryCount:       2 bytes (uint16)
├── Checksum:         4 bytes (uint32, CRC32)
└── Flags:            2 bytes (uint16, reserved)

[Compressed Data: variable]
└── Snappy compressed entries
```

### 5.4 Fájl struktúra

```
[File Header: 64 bytes]
├── Magic:       4 bytes ("HYDR")
├── Version:     2 bytes (currently 2)
├── Flags:       2 bytes
├── CreatedAt:   8 bytes (Unix nano)
├── ModifiedAt:  8 bytes (Unix nano)
├── BlockSize:   4 bytes (16384 = 16KB)
├── EntryCount:  8 bytes
├── BlockCount:  8 bytes
└── Reserved:    16 bytes

[Block 1]
├── Block Header (16 bytes)
└── Compressed Data

[Block 2]
├── Block Header (16 bytes)
└── Compressed Data

...

[Block N]
├── Block Header (16 bytes)
└── Compressed Data
```

---

## 6. Blokkméret és ZFS ajánlások

### 6.1 Alapértelmezett blokkméret

```go
DefaultMaxBlockSize = 16 * 1024  // 16 KB (tömörítés előtt)
```

### 6.2 ZFS beállítások

```bash
# Dataset létrehozása HydrAIDE-hez
zfs create -o recordsize=16K \
           -o compression=off \
           -o atime=off \
           -o primarycache=metadata \
           tank/hydraide

# Magyarázat:
# - recordsize=16K: Illeszkedik a HydrAIDE blokkmérethez
# - compression=off: HydrAIDE már Snappy-t használ
# - atime=off: Nincs access time frissítés (I/O csökkentés)
# - primarycache=metadata: Több RAM az ARC-nak adatokhoz
```

### 6.3 Tömörítési arány

Tipikus Snappy tömörítési arány: **30-70%**

Tehát egy 16KB-os tömörítetlen blokk → 5-11KB tömörítve

---

*Dokumentáció generálva: 2026-01-22*  
*HydrAIDE verzió: v2.1.5*

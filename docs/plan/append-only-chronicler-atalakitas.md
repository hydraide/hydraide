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

### 4.1 Verzió Detektálás

```go
func (c *chronicler) detectVersion(path string) int {
    // Ha folder → Version 1 (régi)
    if isDirectory(path) {
        return 1
    }
    
    // Ha fájl és HYDR magic → Version 2 (új)
    if isFile(path) {
        header := readHeader(path)
        if header.magic == "HYDR" {
            return header.version
        }
    }
    
    return 0 // Nem létezik
}
```

### 4.2 Migráció

```go
func (c *chronicler) Load() {
    version := c.detectVersion(c.path)
    
    switch version {
    case 0:
        // Új swamp, nincs mit betölteni
        return
        
    case 1:
        // Régi formátum - betöltés és konvertálás
        c.loadLegacy()           // Régi módon betölt
        c.migrateToV2()          // Új formátumba ment
        c.deleteLegacyFiles()    // Régi fájlok törlése
        
    case 2:
        // Új formátum
        c.loadV2()
    }
}
```

---

## 5. Implementációs Fázisok

### 5.1 Fázis 1: Alap Infrastruktúra

- [ ] Új `chronicler/v2/` package létrehozása
- [ ] File header struktúra és serialization
- [ ] Block header struktúra és serialization
- [ ] Entry struktúra és serialization
- [ ] Snappy compression integráció

**Becsült idő:** 2-3 nap

### 5.2 Fázis 2: Írási Műveletek

- [ ] Write buffer implementáció
- [ ] Block flush logika
- [ ] Append-only file writer
- [ ] Force flush (shutdown/explicit save)

**Becsült idő:** 2-3 nap

### 5.3 Fázis 3: Olvasási Műveletek

- [ ] File reader és block parser
- [ ] Entry processor (INSERT/UPDATE/DELETE)
- [ ] Fragmentation tracking
- [ ] Index rebuild optimalizáció

**Becsült idő:** 2 nap

### 5.4 Fázis 4: Compaction

- [ ] Compaction algoritmus
- [ ] Observer integráció (shutdown védelem)
- [ ] Atomic file swap
- [ ] Fragmentation threshold konfiguráció

**Becsült idő:** 2 nap

### 5.5 Fázis 5: Migráció és Kompatibilitás

- [ ] Version detection
- [ ] Legacy loader megtartása
- [ ] V1 → V2 migráció
- [ ] Legacy cleanup

**Becsült idő:** 2 nap

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
**18-23 munkanap**

---

## 9. Jóváhagyás

| Név | Szerep | Dátum | Státusz |
|-----|--------|-------|---------|
| Péter | Architect | | ⏳ Függőben |

---

**Következő lépés:** Péter jóváhagyása után a Fázis 1 megkezdése.

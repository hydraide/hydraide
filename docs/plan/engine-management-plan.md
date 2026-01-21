# HydrAIDE Engine Management & Migration Enhancement Plan

## Összefoglaló

Ez a terv leírja a HydrAIDE storage engine kezelésének teljes körű implementációját, beleértve:
- Globális engine beállítás (V1/V2/V3...)
- Teljes mentés és visszaállítás
- Biztonságos migráció
- Méret lekérdezés

---

## 1. Jelenlegi Állapot

### Problémák

1. **Nincs globális engine beállítás** - A `UseChroniclerV2` nem mentődik a `settings.json`-ba
2. **Nincs mentési/visszaállítási funkció** - Manuális `cp -r` szükséges
3. **V1/V2 fájlok konfliktusa** - V1: folder, V2: `.hyd` fájl, mindkettő létezhet
4. **Nincs méret lekérdezés** - Nem tudjuk, mennyi helyet foglal az adatbázis

### Jelenlegi Fájl Struktúra

```
/hydraide/
├── data/                    # Swamp adatok
│   └── words/
│       └── ap/
│           ├── apple/       # V1: folder (chunk-ok + meta.json)
│           │   ├── 1.snappy
│           │   ├── 2.snappy
│           │   └── meta.json
│           └── apple.hyd    # V2: egyetlen fájl (migráció után)
└── settings/
    └── settings.json        # Konfiguráció
```

---

## 2. Javasolt Megoldás

### 2.1 Globális Engine Beállítás

**Új mező a `settings.json`-ban:**

```json
{
  "engine": "V2",
  "patterns": { ... }
}
```

**Engine értékek:**
- `""` vagy `"V1"` → V1 engine (alapértelmezett, visszafelé kompatibilis)
- `"V2"` → V2 append-only engine

**Működési logika:**
1. Ha `engine == "V2"`:
   - Minden swamp a `.hyd` fájlt használja
   - Ha nincs `.hyd` de van V1 folder → hiba (nem migrált!)
2. Ha `engine == ""` vagy `"V1"`:
   - Minden swamp a chunk folder-t használja
   - Ha van `.hyd` fájl → figyelmen kívül hagyja

### 2.2 Új hydraidectl Parancsok

#### `hydraidectl engine` - Engine kezelés

```bash
# Aktuális engine lekérdezése
hydraidectl engine --instance prod

# Engine beállítása (migráció után!)
hydraidectl engine --instance prod --set V2

# Visszaállítás V1-re (rollback esetén)
hydraidectl engine --instance prod --set V1
```

**Működés:**
- Ellenőrzi, hogy az instance létezik
- Ha `--set V2`: ellenőrzi, hogy minden swamp migrálva van-e (van `.hyd`)
- Módosítja a `settings.json` `engine` mezőjét
- Újraindítja az instance-t

#### `hydraidectl backup` - Mentés

```bash
# Teljes mentés
hydraidectl backup --instance prod --target /backup/hydraide-20260121

# Csak adatok mentése (settings nélkül)
hydraidectl backup --instance prod --target /backup/data-only --data-only

# Tömörített mentés
hydraidectl backup --instance prod --target /backup/hydraide.tar.gz --compress
```

**Működés:**
1. Instance megállítása (--no-stop flag-gel kihagyható, de figyelmeztet)
2. Adatok másolása a célmappába
3. settings.json másolása
4. Ellenőrző összegzés (checksum)
5. Instance újraindítása

#### `hydraidectl restore` - Visszaállítás

```bash
# Visszaállítás mentésből
hydraidectl restore --instance prod --source /backup/hydraide-20260121

# Visszaállítás tömörített mentésből
hydraidectl restore --instance prod --source /backup/hydraide.tar.gz

# Visszaállítás force-szal (felülírja a meglévő adatokat)
hydraidectl restore --instance prod --source /backup/hydraide-20260121 --force
```

**Működés:**
1. Instance megállítása
2. Meglévő adatok átnevezése (`.old` suffix)
3. Mentés visszamásolása
4. Ellenőrzés (checksum)
5. Instance újraindítása
6. Sikeres ellenőrzés után `.old` törlése

#### `hydraidectl size` - Méret lekérdezés

```bash
# Teljes méret
hydraidectl size --instance prod

# JSON output
hydraidectl size --instance prod --json

# Részletes (top 10 legnagyobb swamp)
hydraidectl size --instance prod --detailed
```

**Kimenet:**
```
HydrAIDE Instance: prod
=======================

📁 Data Directory:    /var/hydraide/data
📊 Total Size:        45.2 GB
📦 Total Files:       1,234,567
📂 Total Swamps:      15,234

Engine: V2
├── .hyd files:       15,000 (44.8 GB)
└── V1 folders:       234 (0.4 GB) ⚠️ Not migrated!

Top 10 Largest Swamps:
1. words/index          2.3 GB
2. domains/meta         1.8 GB
3. users/profiles       1.2 GB
...
```

### 2.3 Migrate Parancs Bővítése

A jelenlegi `migrate` parancs bővítése:

```bash
# Teljes migráció (megállít, migrál, engine-t V2-re állít, újraindít)
hydraidectl migrate --instance prod --full

# Csak dry-run (instance futhat közben)
hydraidectl migrate --instance prod --dry-run

# Migráció automatikus mentéssel
hydraidectl migrate --instance prod --full --backup /backup/pre-migration
```

**`--full` működés:**
1. Instance megállítása
2. Automatikus mentés (opcionális `--backup` path)
3. V1 → V2 migráció
4. Verifikáció
5. Engine beállítása V2-re
6. V1 fájlok törlése (ha minden OK)
7. Instance újraindítása

### 2.4 Cleanup Parancs

```bash
# V1 fájlok törlése (migráció után)
hydraidectl cleanup --instance prod --v1-files

# V2 fájlok törlése (rollback után)
hydraidectl cleanup --instance prod --v2-files

# Dry-run
hydraidectl cleanup --instance prod --v1-files --dry-run
```

---

## 3. Fájl Struktúra Migrációt Követően

### Migráció Előtt (V1)
```
/data/words/ap/apple/
├── 1.snappy
├── 2.snappy
└── meta.json
```

### Migráció Után (V2)
```
/data/words/ap/apple.hyd    # Egyetlen fájl
```

### Átmeneti Állapot (--keep-original esetén)
```
/data/words/ap/
├── apple/              # V1 (régi, törlésre vár)
│   ├── 1.snappy
│   ├── 2.snappy
│   └── meta.json
└── apple.hyd           # V2 (új, aktív)
```

---

## 4. Engine Detekció és Választás

### Swamp Megnyitáskor

```go
func openSwamp(path string, engine string) {
    v2Path := path + ".hyd"
    v1Path := path + "/"
    
    switch engine {
    case "V2":
        if fileExists(v2Path) {
            return openV2(v2Path)
        }
        if dirExists(v1Path) {
            return error("Swamp not migrated! Run: hydraidectl migrate")
        }
        return createNewV2(v2Path)
        
    case "", "V1":
        if dirExists(v1Path) {
            return openV1(v1Path)
        }
        return createNewV1(v1Path)
    }
}
```

---

## 5. Implementációs Fázisok

### Fázis 1: Settings Bővítés ✅ KÉSZ
- [x] `Model` struct-ba `Engine` mező hozzáadása
- [x] `Settings` interface-be `GetEngine()` / `SetEngine()` / `IsV2Engine()`
- [x] `settings.json` mentés/betöltés frissítése
- [x] Hydra engine-alapú chronicler választás
- [x] **TESZT:** Settings engine mező unit tesztek (4 teszt ✅)

### Fázis 2: hydraidectl engine ✅ KÉSZ
- [x] `engine` parancs implementáció
- [x] `--set` flag engine váltáshoz
- [x] Migráció ellenőrzés V2 beállítás előtt (warning prompt)
- [x] Instance újraindítás engine váltás után
- [x] **TESZT:** Build sikeres ✅

### Fázis 3: hydraidectl backup/restore ⏳
- [ ] `backup` parancs implementáció
- [ ] `restore` parancs implementáció
- [ ] Checksum validáció
- [ ] Tömörítés támogatás (tar.gz)
- [ ] **TESZT:** Backup/restore E2E tesztek

### Fázis 4: hydraidectl size ⏳
- [ ] `size` parancs implementáció
- [ ] V1/V2 szétválasztás
- [ ] Top N legnagyobb swamp listázás
- [ ] **TESZT:** Size parancs unit tesztek

### Fázis 5: Migrate Bővítés ⏳
- [ ] `--full` flag implementáció
- [ ] `--backup` integráció
- [ ] Automatikus engine váltás
- [ ] V1 cleanup migráció után
- [ ] `.migration-lock` fájl kezelés
- [ ] Progress visszajelzés
- [ ] **TESZT:** Migrate --full E2E tesztek

### Fázis 6: Cleanup Parancs ⏳
- [ ] `cleanup` parancs implementáció
- [ ] V1/V2 fájl törlés
- [ ] Dry-run támogatás
- [ ] **TESZT:** Cleanup unit tesztek

### Fázis 7: Dokumentáció ⏳
- [ ] Migration guide frissítés
- [ ] User manual frissítés
- [ ] Példa workflow-ok

### Fázis 8: Végső Tesztelés ⏳
- [ ] **OVERALL TESZT:** Teljes rendszer teszt futtatás
- [ ] **E2E:** Teljes migrációs workflow teszt
- [ ] **E2E:** Teljes rollback workflow teszt
- [ ] **E2E:** Backup → Migrate → Restore cycle teszt
- [ ] Minden teszt ZÖLD ellenőrzés

---

## 6. Teljes Migrációs Workflow

```
┌─────────────────────────────────────────────────────────────┐
│                    MIGRÁCIÓ WORKFLOW                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. ELŐKÉSZÍTÉS                                             │
│     ┌──────────────────────────────────────────────────┐    │
│     │ hydraidectl backup --instance prod \             │    │
│     │              --target /backup/pre-migration      │    │
│     └──────────────────────────────────────────────────┘    │
│                           │                                 │
│                           ▼                                 │
│  2. VALIDÁCIÓ (Dry-Run)                                     │
│     ┌──────────────────────────────────────────────────┐    │
│     │ hydraidectl migrate --instance prod --dry-run    │    │
│     └──────────────────────────────────────────────────┘    │
│                           │                                 │
│                           ▼                                 │
│  3. MIGRÁCIÓ                                                │
│     ┌──────────────────────────────────────────────────┐    │
│     │ hydraidectl migrate --instance prod --full       │    │
│     │                                                  │    │
│     │ Ez automatikusan:                                │    │
│     │   - Megállítja az instance-t                     │    │
│     │   - Migrál V1 → V2                               │    │
│     │   - Beállítja engine = "V2"                      │    │
│     │   - Törli a V1 fájlokat                          │    │
│     │   - Újraindítja az instance-t                    │    │
│     └──────────────────────────────────────────────────┘    │
│                           │                                 │
│                           ▼                                 │
│  4. ELLENŐRZÉS                                              │
│     ┌──────────────────────────────────────────────────┐    │
│     │ hydraidectl size --instance prod                 │    │
│     │ hydraidectl health --instance prod               │    │
│     └──────────────────────────────────────────────────┘    │
│                           │                                 │
│                           ▼                                 │
│  5. KÉSZ! ✅                                                │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 7. Rollback Workflow

```
┌─────────────────────────────────────────────────────────────┐
│                    ROLLBACK WORKFLOW                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. INSTANCE MEGÁLLÍTÁSA                                    │
│     ┌──────────────────────────────────────────────────┐    │
│     │ hydraidectl stop --instance prod                 │    │
│     └──────────────────────────────────────────────────┘    │
│                           │                                 │
│                           ▼                                 │
│  2. VISSZAÁLLÍTÁS MENTÉSBŐL                                 │
│     ┌──────────────────────────────────────────────────┐    │
│     │ hydraidectl restore --instance prod \            │    │
│     │              --source /backup/pre-migration      │    │
│     │                                                  │    │
│     │ Ez automatikusan:                                │    │
│     │   - Törli a V2 fájlokat                          │    │
│     │   - Visszaállítja a V1 fájlokat                  │    │
│     │   - Beállítja engine = "V1"                      │    │
│     │   - Újraindítja az instance-t                    │    │
│     └──────────────────────────────────────────────────┘    │
│                           │                                 │
│                           ▼                                 │
│  3. ELLENŐRZÉS                                              │
│     ┌──────────────────────────────────────────────────┐    │
│     │ hydraidectl health --instance prod               │    │
│     └──────────────────────────────────────────────────┘    │
│                           │                                 │
│                           ▼                                 │
│  4. V1-EN VISSZAÁLLT ✅                                     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 8. Kérdések / Döntési Pontok

### Q1: Mi történjen, ha V2 engine van beállítva, de van nem migrált V1 swamp?

**Döntés: ✅ KRITIKUS HIBA!** 
- A rendszer NE nyissa meg a swamp-ot
- Az adatbázist le KELL állítani
- Komoly hibajelzés szükséges, mert ez inkonzisztenciához vezethet
- A felhasználónak migrálnia kell a swamp-ot

### Q2: Lehet-e swamp-onként különböző engine?

**Döntés: ✅ NEM!** 
- Egy instance-on belül MINDIG egyféle engine működik
- Csak teljes migráció és átállás lehetséges
- Ez egyszerűsíti a kezelést és elkerüli a keveredést

### Q3: Mi a backup formátum?

**Döntés: ✅ ELFOGADVA**
- Alapértelmezés: egyszerű mappa másolat (`cp -r`)
- Opcionális: tömörített tar.gz (`--compress`)
- A settings.json és a data mappa is mentődik
- Olyan megoldás kell, ami a legtöbb rendszeren működik és gyorsan képes fájlokat másolni/tömöríteni

### Q4: Kell-e lock fájl migráció alatt?

**Döntés: ✅ IGEN, KÖTELEZŐ!**
- Egy `.migration-lock` fájl a data mappában
- Két migráció NE indulhasson el egyszerre
- Migráció ideje alatt teljes visszajelzés szükséges a progress-ről

---

## 9. Becsült Időráfordítás

| Fázis | Feladat | Idő |
|-------|---------|-----|
| 1 | Settings bővítés | 2-3 óra |
| 2 | hydraidectl engine | 3-4 óra |
| 3 | hydraidectl backup/restore | 6-8 óra |
| 4 | hydraidectl size | 2-3 óra |
| 5 | Migrate bővítés | 4-5 óra |
| 6 | Cleanup parancs | 2-3 óra |
| 7 | Dokumentáció | 2-3 óra |
| **Összesen** | | **~25-30 óra** |

---

## 10. Elfogadás

- [x] Péter jóváhagyása ✅ (2026-01-21)
- [ ] Implementáció indítása

**Terv státusz:** ✅ JÓVÁHAGYVA - Implementáció folyamatban

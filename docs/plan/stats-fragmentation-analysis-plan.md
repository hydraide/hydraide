# Stats & Fragmentáció Elemzés és Javítási Terv (v2)

## 📋 Összefoglaló

**Cél**: 
1. Debuggolni a frissen migrált adatbázis váratlan fragmentációját (31.2%)
2. Stats parancs bővítése (swamp név, blokk statisztikák)
3. Új debug parancs swamp tartalmának vizsgálatára
4. Új compact parancs implementálása

**Érintett komponensek**:
- `app/hydraidectl/cmd/stats.go` - Stats parancs bővítése
- `app/hydraidectl/cmd/inspect.go` - (ÚJ) Swamp debug/inspect parancs
- `app/hydraidectl/cmd/compact.go` - (ÚJ) Compaction parancs
- `app/core/hydra/swamp/chronicler/v2/reader.go` - Reader bővítése (ha szükséges)

---

## 🔍 Tisztázott pontok

1. **V1-ben 1 kulcs = 1 előfordulás**: Tehát a duplikáció NEM okozhatja a fragmentációt
2. **V1-ben valós törlés volt**: DELETE entry-k NEM keletkezhettek a migrációból
3. **A dead entry-k eredete ismeretlen** - debuggolni kell!

---

## ✅ Megoldási Terv

### 1. Fázis: Stats parancs bővítése

**Cél**: Minden swamphoz megjeleníteni a valódi nevét a metaadatból

- [x] `analyzeSwamp` függvény módosítása: `LoadIndex` hívás a swamp név kiolvasásához
- [x] `SwampStats` struct bővítése: `SwampName` mező (a path mellett)
- [x] Output formázás frissítése: swamp név megjelenítése (ha van)
- [ ] Blokkok számának és avg entries/block megjelenítése (TODO - következő iteráció)

**Fázis státusz:** ✅ Kész

---

### 2. Fázis: Swamp Inspect/Debug parancs

**Cél**: Egy swamp teljes tartalmának vizsgálata debuggolás céljából

**Parancs**: `hydraidectl inspect --instance <name> --swamp <path>`

**Funkciók**:
- [x] Swamp fájl megnyitása és header információk megjelenítése
- [x] Összes entry listázása sorban:
  - Entry sorszám
  - Operation (INSERT/UPDATE/DELETE/METADATA)
  - Kulcs
  - Data méret (GOB méret bájtban)
  - Timestamp-ek (ha elérhetők a GOB-ból)
- [x] Lapozás támogatása (`--page`, `--per-page`)
- [x] JSON export (`--json --output <file>`)
- [x] Összefoglaló: összes entry, live, dead, fragmentáció

**GOB tartalom részleges kiolvasása**:
- [x] `Key` - kulcs
- [x] `CreatedAt` / `UpdatedAt` - időbélyegek
- [x] `CreatedBy` / `UpdatedBy` - létrehozó/módosító
- [x] `ExpireAt` - lejárat
- [x] Data méret (a tényleges payload hossza)

**Fázis státusz:** ✅ Kész

---

### 3. Fázis: Compact parancs

**Cél**: Instance leállítása, compaction futtatása, opcionális újraindítás

**Parancs**: `hydraidectl compact --instance <name> [--parallel 4] [--restart]`

**Funkciók**:
- [x] Instance leállítása (ha fut)
- [x] Összes V2 swamp begyűjtése
- [x] Párhuzamos compaction worker pool-lal
- [x] Progress bar megjelenítése
- [x] Végső jelentés:
  - Hány swamp lett compactolva
  - Mennyi hely szabadult fel
  - Előtte/utána fragmentáció
- [x] `--restart` flag: instance újraindítása compaction után
- [x] `--dry-run` flag: csak jelentés, tényleges compaction nélkül

**Fázis státusz:** ✅ Kész

---

### 4. Fázis: Dokumentáció

- [ ] `hydraidectl inspect` parancs dokumentálása
- [ ] `hydraidectl compact` parancs dokumentálása
- [ ] Stats parancs frissített dokumentálása

**Fázis státusz:** ⏳ Várakozik

---

### 5. Fázis: CHANGELOG

- [ ] `docs/changelogs/2026-01-22.md` frissítése

**Fázis státusz:** ⏳ Várakozik

---

## 📏 Blokkméret információ

```go
DefaultMaxBlockSize = 16 KB (tömörítés előtti, uncompressed)
```

**ZFS ajánlás**: `recordsize=16K` a HydrAIDE dataset-ekre

---

## 🔧 Debug workflow

1. **Először**: Futtatjuk az `inspect` parancsot a 100%-os fragmentációjú swampon
2. **Megnézzük**: Mi az a dead entry - DELETE, duplikált kulcs, vagy valami más?
3. **Ha megértjük**: Javítjuk a migrátort vagy a fragmentáció számítást
4. **Végül**: Compact parancs tesztelése

---

## 🚀 Használat

### Inspect parancs (debug):
```bash
# Terminál output:
hydraidectl inspect --instance t-outbound-test --swamp 875/99a/99aa514918c642e3

# JSON export fájlba:
hydraidectl inspect --instance t-outbound-test --swamp 875/99a/99aa514918c642e3 --json --output debug.json
```

### Stats parancs (most már swamp névvel):
```bash
hydraidectl stats --instance t-outbound-test
```

### Compact parancs:
```bash
# Dry-run (csak elemzés):
hydraidectl compact --instance t-outbound-test --dry-run

# Tényleges compaction:
hydraidectl compact --instance t-outbound-test --parallel 4

# Compaction + újraindítás:
hydraidectl compact --instance t-outbound-test --parallel 4 --restart
```

---

*Generálva: 2026-01-22*
*Verzió: 2*
*Státusz: Implementáció kész, tesztelésre vár*

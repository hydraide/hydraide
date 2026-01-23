# HydrAIDE Observe Fejlesztési Terv

## Összefoglaló

A cél az observe parancs fejlesztése az alábbi funkciókkal:
1. **FailedPrecondition külön kezelése** - nem hiba, hanem INFO státusz
2. **SwampPath hozzáadása a telemetriához** - inspect funkcióhoz szükséges
3. **Interaktív swamp inspect** az observer-en belül

## Érintett komponensek

- `proto/hydraide.proto` - TelemetryEvent kiegészítése SwampPath mezővel
- `app/server/server/server.go` - SwampPath kinyerése és küldése
- `app/server/telemetry/telemetry.go` - Event struktúra bővítése
- `app/server/gateway/gateway.go` - SwampPath továbbítása
- `app/hydraidectl/cmd/observe/model.go` - TUI logika módosítása
- `app/hydraidectl/cmd/observe/styles.go` - Új stílusok (INFO/warning)

---

## 1. Fázis: FailedPrecondition kezelése külön státuszként

### Cél
A `FailedPrecondition` hibakód (pl. "Swamp does not exist") nem valódi hiba, hanem informatív válasz. Külön kell kezelni:
- Sárga/narancssárga `⚠` jellel megjeleníteni
- Errors tab-ban NE jelenjen meg
- Stats-ban külön "Precondition Warnings" szekció

### Lépések

- [x] **styles.go**: Új `warningStyle` hozzáadása (sárga szín)
- [x] **model.go**: `renderEventRow` - FailedPrecondition esetén `⚠ INFO` megjelenítés
- [x] **model.go**: `renderErrorsTab` - FailedPrecondition szűrése (ne jelenjen meg)
- [x] **model.go**: `renderStatsTab` - Külön "Precondition Warnings" vs "Real Errors" szekció
- [x] **model.go**: `renderStatusBar` - Külön számláló real errors és info-nak
- [x] **model.go**: Island ID eltávolítása a swamp névből (pl. 193/... -> ...)

**Fázis státusz:** ✅ Kész

---

## 2. Fázis: SwampPath hozzáadása a telemetriához

### Cél
Az observe-ból közvetlenül meg lehessen nyitni egy swamp-ot inspect-tel. Ehhez szükséges a swamp fájl útvonala.

### Probléma
Jelenleg a `SwampName` tartalmazza az island ID-t + swamp nevet (pl. `193/queueService/catalog/gptQueue`), de NEM tartalmazza a tényleges fájl path-ot (pl. `193/a2b/a2b3c4d5e6f7.hyd`).

### Megoldás
A szerver a swamp műveletek során tudja a tényleges fájl path-ot. Ezt hozzá kell adni a telemetry event-hez.

### Lépések

- [ ] **proto**: `TelemetryEvent`-hez új mező: `string swamp_path = 14;`
- [ ] **proto regenerálás**: `make proto`
- [ ] **telemetry.go**: `Event` struktúra bővítése `SwampPath string` mezővel
- [ ] **server.go**: `extractSwampPath` függvény létrehozása (a core-ból kinyerni)
- [ ] **gateway.go**: SwampPath továbbítása a proto event-be
- [ ] **observe model.go**: SwampPath tárolása az Event-ben

**Fázis státusz:** ❌ Még nem kezdődött

---

## 3. Fázis: Interaktív Swamp Inspect az Observer-ben

### Cél
Pause módban kiválasztott sorra Enter-t nyomva megnyílik a swamp tartalma egy új nézetben.

### Működés
1. Felhasználó pausolja az observer-t (P)
2. Nyilakkal kiválaszt egy sort
3. Enter-rel megnyitja
4. Új nézet: Swamp tartalom (GetAll gRPC hívás)
5. ESC-kel visszalép az observer-be

### Megjelenítendő adatok (GetAll válaszból)
- Key
- Value típus és érték (primitív) VAGY "bytes (X KB)" ha byte tömb
- CreatedAt, CreatedBy
- UpdatedAt, UpdatedBy
- ExpiredAt

### Lépések

- [ ] **model.go**: Új állapot `showSwampDetail bool` és `selectedSwampData []Treasure`
- [ ] **model.go**: Enter key kezelése - gRPC `GetAll` hívás a kiválasztott swamp-ra
- [ ] **model.go**: `renderSwampDetail()` függvény létrehozása
- [ ] **model.go**: ESC kezelése - visszalépés a listához
- [ ] **styles.go**: Swamp detail stílusok

**Fázis státusz:** ❌ Még nem kezdődött

---

## 4. Fázis: Dokumentáció és CHANGELOG

- [ ] README frissítése az új observe funkciókkal
- [ ] hydraidectl user manual frissítése
- [ ] CHANGELOG bejegyzés

**Fázis státusz:** ❌ Még nem kezdődött

---

## Kérdések Péternek - MEGVÁLASZOLVA

1. **SwampPath kinyerése**: A `name` csomag `GetFullHashPath` függvénye determinisztikusan kiszámítja a path-ot a swamp nevéből. Szükséges paraméterek:
   - `rootPath` = data folder path
   - `islandID` = az island azonosító (a SwampName elején lévő szám)
   - `depth` = GetHashFolderDepth() - settings-ből
   - `maxFoldersPerLevel` = GetMaxFoldersPerLevel() - settings-ből
   
   **Nem kell a szervernek küldenie** - kliens oldalon számítható!

2. **Inspect hívás**: `GetByIndex` használandó lapozással (nem GetAll), mert nagy swamp-ok lehetnek (akár 2M rekord).

3. **Island ID megjelenítés**: A listában NEM kell, csak az inspect-nél fontos.

---

## Következő lépés

Kezdem az 1. Fázissal (FailedPrecondition kezelés).

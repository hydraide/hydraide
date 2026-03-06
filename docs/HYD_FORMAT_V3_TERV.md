# HydrAIDE .hyd Fajlformatum V3 — Extended Header + Migrator

## 1. Osszefoglalo

**Cel:** A `.hyd` fajlformatum bovitese, hogy a swamp neve kozvetlenul a header utan legyen tarolva (nem compressed block-ban), igy millios nagyságrendu fajlnal is pillanatok alatt kiolvashatoak a nevek block dekompresszio nelkul.

**Erintett komponensek:**
- `app/core/hydra/swamp/chronicler/v2/types.go` — FileHeader bovites (V3)
- `app/core/hydra/swamp/chronicler/v2/writer.go` — iras logika modositas
- `app/core/hydra/swamp/chronicler/v2/reader.go` — olvasas logika modositas
- `app/core/hydra/swamp/chronicler/v2/compactor.go` — compaction frissites
- `app/core/hydra/swamp/chronicler/chronicler_v2.go` — adapter frissites
- `app/core/hydra/swamp/chronicler/v2/migrator/` — V2→V3 migrator

---

## 2. Jelenlegi allapot (V2 formatum)

```
[FileHeader 64B] [Block1] [Block2] ... [BlockN]
```

- FileHeader: fix 64 byte, tartalmazza: Magic, Version(2), Flags, CreatedAt, ModifiedAt, BlockSize, EntryCount, BlockCount, Reserved[16]
- A swamp neve `OpMetadata` entry-kent van az elso compressed block-ban (`__swamp_meta__` kulcs)
- A nev kiolvasasahoz: header olvasas + block header olvasas + block dekompresszio + entry parse

**Problema:** Milliok fajlnal a block dekompresszio fajlonkent felesleges overhead, ha csak a nevet akarjuk kiolvasni.

---

## 3. Uj formatum (V3)

```
[FileHeader 64B] [SwampNameLen 2B] [SwampName NB] [Block1] [Block2] ... [BlockN]
```

### 3.1. FileHeader valtozasok

```go
type FileHeader struct {
    Magic      [4]byte   // "HYDR" — valtozatlan
    Version    uint16    // 2 → 3
    Flags      uint16    // valtozatlan
    CreatedAt  int64     // valtozatlan
    ModifiedAt int64     // valtozatlan
    BlockSize  uint32    // valtozatlan
    EntryCount uint64    // valtozatlan
    BlockCount uint64    // valtozatlan
    NameLength uint16    // UJ: a swamp nev hossza byte-ban
    Reserved   [14]byte  // 16 → 14 byte (2 byte-ot elhasznaltunk)
}
```

A `Reserved` mezobol 2 byte-ot hasznalunk el a `NameLength`-hez. A header merete valtozatlanul **64 byte** marad.

### 3.2. Nev tarolasa

Kozvetlenul a 64 byte-os header utan:
- `NameLength` byte-nyi adat = a swamp teljes neve UTF-8 kodolasban
- Peldaul: `"users/profiles/user_123"` = 23 byte

A block-ok ezutan kovetkeznek: a `FileHeaderSize + NameLength` offset-tol.

### 3.3. Block-okbol a metadata entry torlese

V3 fajlokban az `OpMetadata` entry (`__swamp_meta__` kulcs) **nem kerul be** a block-okba.
A migracio soran a meglevo `OpMetadata` entry-k ki lesznek szurve.

---

## 4. Visszafele kompatibilitas

### Olvasas
- V3 reader: megnezi a `Version` mezot
  - Ha `Version == 2`: a nevet a block-okbol olvassa (regi mod, mint eddig)
  - Ha `Version == 3`: a nevet a header utani mezobol olvassa
- Igy a rendszer V2 es V3 fajlokat egyarant kepes olvasni

### Iras
- Uj fajlok mindig V3 formatumban jnnek letre
- V2 fajlok V3-ra frissulnek:
  - Automatikusan compaction soran, VAGY
  - Manualis migracio-val (hydraidectl parancs)

### A CurrentVersion konstans
```go
// V2 marad tamogatott olvasasra, de uj fajlok V3-ban jonnek letre
const (
    Version2 uint16 = 2
    Version3 uint16 = 3
    CurrentVersion = Version3  // uj fajlok ezzel jonnek letre
)
```

A `Deserialize`-ben a version check:
```go
if h.Version != Version2 && h.Version != Version3 {
    return ErrUnsupportedVer
}
```

---

## 5. Erintett fajlok reszletes valtozasai

### 5.1. `v2/types.go`

- [ ] `FileHeader` struct: `Reserved [16]byte` → `NameLength uint16` + `Reserved [14]byte`
- [ ] `FileHeaderSize` marad 64
- [ ] `CurrentVersion` 2 → 3
- [ ] `Serialize()` / `Deserialize()` frissites a `NameLength` mezovel
- [ ] Version check: V2 es V3 is elfogadott
- [ ] `SwampMetadata` struct es serialize/deserialize megtartasa (V2 kompatibilitas)
- [ ] Uj konstans: `Version2 = 2`, `Version3 = 3`

### 5.2. `v2/writer.go`

- [ ] `createNewFile()`: header utan kiirja a swamp nevet (`NameLength` + name bytes)
- [ ] `NewFileWriter()`: swamp nev parameterkent
- [ ] `openExistingFile()`: V3 fajlnal atugorja a nev szekciot a block-ok elejehez
- [ ] V3 fajloknal NEM ir `OpMetadata` entry-t a block-okba

### 5.3. `v2/reader.go`

- [ ] `NewFileReader()`: V3 fajlnal beolvassa a nevet a header utan
- [ ] `GetSwampName() string` uj metodus
- [ ] `ReadAllEntries()`: seek pozicio V3-nal `FileHeaderSize + NameLength`
- [ ] `LoadIndex()`: V3-nal a nevet a reader-bol veszi, nem az `OpMetadata`-bol
- [ ] V2 fajloknal a regi logika marad (block-bol olvassa)

### 5.4. `v2/compactor.go`

- [ ] Compaction soran V2 fajl → V3 fajlt ir (automatikus upgrade)
- [ ] `OpMetadata` entry-ket kiszuri a block-okbol V3 iras eseten

### 5.5. `chronicler_v2.go`

- [ ] `NewV2()` / `NewV2WithSwampName()`: swamp nevet atadja a writer-nek
- [ ] `Load()`: a reader-bol olvassa a nevet (nem a block-bol)
- [ ] `Write()`: V3 mod, nem ir `OpMetadata` entry-t

### 5.6. Uj: `ReadSwampName()` segédfuggveny

Gyors, kulon fuggveny ami CSAK a nevet olvassa ki egy .hyd fajlbol:

```go
// ReadSwampName reads only the swamp name from a .hyd file.
// For V3: reads 64B header + 2B name length + name bytes (no block decompression).
// For V2: falls back to reading blocks until OpMetadata is found.
func ReadSwampName(filePath string) (string, error)
```

Ez a fuggveny lesz a scanner lelke — fajlonkent ~100 byte olvasas V3 eseten.

---

## 6. Migrator (V2 → V3)

### 6.1. Mukodes

Minden .hyd fajlra:
1. Megnyitja a fajlt, olvassa a header-t
2. Ha mar V3 → kihagyja (skip)
3. Ha V2:
   a. Vegigolvassa az osszes block-ot es entry-t (dekompresszalas → nyers entry-k)
   b. Megkeresi az `OpMetadata` entry-t → kinyeri a swamp nevet
   c. Letrehoz egy uj temp fajlt (`.hyd.tmp`) V3 formatumban:
      - Uj header (Version=3, NameLength kitoltve)
      - Swamp nev kozvetlenul a header utan (plain text, NEM tomoritett)
      - Block-ok ujrairasa: nyers entry-kbol uj block-ok epitese → tomoritest
        FONTOS: az entry.Data NYERS (mar dekompresszalt) adatot tartalmaz,
        a writer ujra tomoriti block szinten. Nincs dupla tomoritest!
      - `OpMetadata` entry-k KIHAGYASA a block-okbol
   d. **VALIDACIO**: a temp fajl visszaolvasasa es ellenorzese:
      - Header olvasas: Magic == "HYDR", Version == 3, NameLength helyes
      - Swamp nev olvasas: megegyezik az eredetivel
      - Osszes entry visszaolvasasa: darabszam egyezik az eredeti live entry-kkel
      - Ha BARMI hiba → temp fajl torlese, regi fajl sertetlen marad, hiba logolas
   e. CSAK HA a validacio sikeres → atomikus `os.Rename(.hyd.tmp → .hyd)`
   f. Ha a rename sikertelen → temp fajl torlese, hiba logolas

### 6.2. Dupla tomoritest elkerulese

A jelenlegi V2 flow:
```
.hyd fajl → [compressed block] → Snappy decompress → nyers entry-k (Data = nyers byte-ok)
```

A migracio soran:
```
nyers entry-k → uj block-okba csoportositas → Snappy compress → uj .hyd fajl
```

Az `entry.Data` byte-ok mindig NYERS (dekompresszalt) allapotban vannak az entry parse utan.
A tomoritest a block writer vegzi, NEM az entry szinten. Igy nincs dupla tomoritest.

A kodban figyelni kell, hogy:
- NE hivjunk `snappy.Encode()` -ot az `entry.Data`-ra kulon
- A `FileWriter.WriteEntries()` mar maga vegzi a block szintu tomoriteest
- A compactor.go-ban mar igy mukodik — ugyan ezt a mintat kell kovetni

### 6.3. Parhuzamossag
- Worker pool (`runtime.NumCPU()` goroutine, `panichandler.SafeGo`-val)
- Island-onkent (1-1000) kulon taskok → channel-en atkuldve a workereknek
- Progress counter atomikus frissitessel

### 6.4. Biztonsag
- Temp fajl irasa `.hyd.tmp` kiterjesztessel
- A regi fajl CSAK AKKOR torlodik, ha az uj fajl VISSZAOLVASASA SIKERES
- Validacios lepeesek: header + nev + entry darabszam ellenorzes
- `--dry-run` flag: csak megszamolja hany fajlt kell migalni, de nem modosit semmit
- Ha a szerver fut: a migracio a szerver kontextusaban tortenik (swamp lock vedelem)

### 6.4. hydraidectl parancs

```bash
# Dry-run: megszamolja a V2 fajlokat
hydraidectl migrate-v3 --instance prod --dry-run

# Migracio futtatasa
hydraidectl migrate-v3 --instance prod --workers 8

# Vagy a meglevo migrate parancsba integralva:
hydraidectl migrate --instance prod --target v3 --dry-run
hydraidectl migrate --instance prod --target v3 --workers 8
```

**Kimenet:**
```
Scanning for V2 files...
Found 1,234,567 V2 files to migrate.
Migrating with 8 workers...
[========================================] 1,234,567 / 1,234,567 (100%)
Migration complete in 3m 42s.
  - Migrated: 1,234,560
  - Skipped (already V3): 0
  - Errors: 7
```

---

## 7. Fazisokra bontott megvalositas

### 1. Fazis: Fajlformatum V3 (types + reader + writer)
- [ ] `v2/types.go` — FileHeader bovites (NameLength, Version3 konstans)
- [ ] `v2/types.go` — Serialize/Deserialize frissites V2+V3 tamogatassal
- [ ] `v2/writer.go` — V3 formatum iras (nev a header utan, OpMetadata kihagyasa)
- [ ] `v2/reader.go` — V3 formatum olvasas + `GetSwampName()` + `ReadSwampName()`
- [ ] `v2/compactor.go` — V2→V3 automatikus upgrade compaction soran
- [ ] `chronicler_v2.go` — adapter frissites az uj reader/writer-hez
- [ ] Unit testek: V3 iras/olvasas, V2 visszafele kompatibilitas, mixed V2+V3

**Fazis statusz:** ⏳

### 2. Fazis: Migrator
- [ ] `v2/migrator/` — V2→V3 migralo logika (fajlonkenti atiras)
- [ ] Parhuzamos worker pool
- [ ] Atomikus fajlcsere (temp file + rename)
- [ ] Dry-run tamogatas
- [ ] Unit testek: migracio, hibakezelés, V3 kihagyas
- [ ] `hydraidectl` parancs integracio

**Fazis statusz:** ⏳

### 3. Fazis: Teszteles (lasd reszletesen lent, 8. szekció)
- [ ] Minden teszt kategoria implementalasa
- [ ] Minden teszt zolden fut

**Fazis statusz:** ⏳

---

## 8. Tesztelesi terv

Ez a szekció definialja az osszes tesztet, aminek ZOLDEN KELL FUTNIA mielott barmit eles kornyezetbe viszunk.

Futtatasi parancs az osszes teszthez:
```bash
go test ./app/core/hydra/swamp/chronicler/v2/... -v -count=1
go test ./app/core/hydra/swamp/chronicler/... -v -count=1
```

### 8.1. FileHeader V3 — Unit testek (`v2/types_test.go`)

**Cel:** A V3 header serialize/deserialize hibatlanul mukodik, es V2-vel visszafele kompatibilis.

```
TestV3Header_SerializeDeserialize
    V3 header letrehozasa NameLength=25 ertekkel
    → Serialize → Deserialize → minden mezo egyezik
    → A header merete pontosan 64 byte

TestV3Header_NameLengthPreserved
    Kulonbozo NameLength ertekek (0, 1, 100, 255, 65535)
    → Serialize → Deserialize → NameLength megmarad

TestV2Header_StillReadable
    V2 header (Version=2, Reserved[16] csupa nulla)
    → Deserialize sikeres, Version==2, NameLength==0

TestV3Header_InvalidMagic
    "XXXX" magic byte-ok → ErrInvalidMagic

TestV3Header_BufferTooSmall
    63 byte-os buffer → hiba

TestV3Header_Version2And3Accepted
    Version=2 → sikeres
    Version=3 → sikeres
    Version=4 → ErrUnsupportedVer
    Version=1 → ErrUnsupportedVer
```

### 8.2. Writer V3 — Unit testek (`v2/writer_test.go`)

**Cel:** A V3 writer helyesen irja ki a nevet a header utan, es a block-ok a nev utan kovetkeznek.

```
TestV3Writer_CreatesFileWithName
    Uj fajl letrehozasa "users/profiles/peter" nevvel
    → Fajl letezik
    → Elso 64 byte: valid V3 header, NameLength==20
    → Kovetkezo 20 byte: "users/profiles/peter"

TestV3Writer_WriteEntries_AfterName
    Fajl letrehozasa nevvel + 10 entry iras
    → Reader-rel visszaolvasva: mind a 10 entry megvan es helyes
    → A nev is kiolvashatoi

TestV3Writer_EmptyName
    Ures nev ("") → NameLength==0, block-ok kozvetlenul a header utan

TestV3Writer_LongName
    Nagyon hosszu nev (1000 karakter)
    → NameLength==1000, a nev teljes egeszeben kiolvashatoi

TestV3Writer_NoOpMetadataInBlocks
    V3 fajl irasanal NEM szabad OpMetadata entry-t irni a block-okba
    → Osszes entry visszaolvasasa: egyik sem OpMetadata

TestV3Writer_DataNotDoubleCompressed
    Entry irasa ismert nyers adattal (pl. "hello world" bytes)
    → Visszaolvasas utan az adat PONTOSAN megegyezik
    → Ha dupla tomoritest lenne, a visszaolvasott adat hibas lenne
```

### 8.3. Reader V3 — Unit testek (`v2/reader_test.go`)

**Cel:** A V3 reader a nevet a header utanrol olvassa, es V2 fajlokat is tud kezelni.

```
TestV3Reader_ReadsNameFromHeader
    V3 fajl megnyitasa → GetSwampName() → helyes nev

TestV3Reader_ReadsEntriesCorrectly
    V3 fajl 50 entry-vel → ReadAllEntries → mind az 50 megvan, adatok helyesek

TestV3Reader_LoadIndex_V3
    V3 fajl → LoadIndex() → helyes index + helyes swamp nev
    → Nincs OpMetadata az index-ben

TestV2Reader_Fallback
    V2 fajl (OpMetadata a block-ban) → GetSwampName() → helyes nev
    → LoadIndex() → helyes index + helyes swamp nev

TestV3Reader_ReadSwampName_Fast
    ReadSwampName() fuggveny V3 fajlon
    → Helyes nev
    → A fuggveny NEM olvassa be az osszes block-ot (teljesitmeny check)

TestV3Reader_ReadSwampName_V2Fallback
    ReadSwampName() fuggveny V2 fajlon
    → Helyes nev (block-bol olvassa)
```

### 8.4. Round-trip teszt (`v2/roundtrip_v3_test.go`)

**Cel:** Adat integritast biztositas — amit beirunk, azt PONTOSAN ugyanugy kapjuk vissza.

```
TestV3RoundTrip_SingleEntry
    1 entry iras V3-ba → visszaolvasas → key, data, operation egyezik BIT-RE PONTOSAN

TestV3RoundTrip_ManyEntries
    1000 entry iras (kulonbozo meretu data: 0B, 1B, 100B, 10KB, 64KB)
    → Visszaolvasas → MINDEN entry egyezik

TestV3RoundTrip_MixedOperations
    Insert + Update + Delete muveletek sorozata
    → LoadIndex() → az elvart live entry-k maradnak, a toroltek eltunnek

TestV3RoundTrip_BinaryData
    Entry-k random binaris adattal (nem csak szoveg)
    → Visszaolvasas → byte-ra pontos egyezes

TestV3RoundTrip_UnicodeSwampName
    Swamp nev ekezetes/unicode karakterekkel: "teszt/álom/swämp"
    → Visszaolvasas → a nev PONTOSAN megegyezik

TestV3RoundTrip_WriteCloseReopenRead
    Iras → Close → uj Reader nyitas → olvasas → minden helyes
    (nem csak memory-ben, hanem tenylegesen lemezrol)
```

### 8.5. Visszafele kompatibilitas teszt (`v2/compat_v3_test.go`)

**Cel:** A V3 kod SEMMIT NEM ront el a meglevo V2 fajlokon.

```
TestCompat_V2FileCreatedByOldCode
    Legyartunk egy V2 fajlt a JELENLEGI (V2) koddal (meg a modositas elott)
    → Elmentjuk testdata-kent
    → A V3 reader HIBA NELKUL olvassa
    → Minden entry helyes
    → A swamp nev a block-bol kiolvashatoi

TestCompat_V2FileReadWrite_NoCorruption
    V2 fajl megnyitasa → uj entry hozzaadasa a V3 writer-rel
    → A fajl V3-ra frissul
    → A regi entry-k MIND megvannak + az uj is

TestCompat_V3FileNotReadableByV2Code
    Ez egy DOKUMENTACIOS teszt — verifikaljuk hogy a V2 reader
    a Version=3 fajlon ErrUnsupportedVer hibat ad
    (igy ha valaki regi szervert hasznal, nem korruptaóodik az adat)

TestCompat_MixedV2V3Directory
    Egy mappaban V2 es V3 fajlok vegyesen
    → ReadSwampName() mindkettobol helyes nevet ad
```

### 8.6. Migrator teszt (`v2/migrator/migrator_v3_test.go`)

**Cel:** A V2→V3 migracio hibatlan, adatvesztes nelkuli, es biztonsagos.

```
TestMigrator_V2toV3_BasicMigration
    V2 fajl 10 entry-vel + OpMetadata
    → Migracio → V3 fajl
    → Header: Version==3, NameLength helyes
    → Nev: header utanrol olvashatoi, helyes
    → Entry-k: mind a 10 megvan, adatok bit-re pontosak
    → OpMetadata: NINCS a block-okban

TestMigrator_V2toV3_NoDataLoss
    V2 fajl 10000 kulonbozo entry-vel (insert, update, delete vegyesen)
    → Migracio
    → LoadIndex() az uj fajlon → PONTOSAN ugyan azok a live key-ek es adatok

TestMigrator_V2toV3_OpMetadataRemoved
    Migralt V3 fajl → osszes entry olvasasa
    → Egyetlen OpMetadata entry sem talalhato a block-okban

TestMigrator_V3Skipped
    V3 fajl → migracio → skip (nem modositja)
    → A fajl VALTOZATLAN (byte-ra azonos)

TestMigrator_ValidationCatchesCorruption
    V2 fajl migracio, de a temp fajl irasa kozben szimulalt hiba
    → A regi fajl SERTETLEN marad
    → A .hyd.tmp fajl torlodik

TestMigrator_AtomicRename
    Sikeres migracio → nincs .hyd.tmp fajl
    → Csak a .hyd fajl letezik, V3 formatumban

TestMigrator_TempFileCleanup_OnError
    Hibas migracio (pl. disk full szimulacio)
    → .hyd.tmp fajl nem marad haatra
    → Eredeti .hyd fajl sertetlen

TestMigrator_ParallelMigration
    100 V2 fajl → 4 worker-es parhuzamos migracio
    → Mind a 100 fajl V3 formatumu lesz
    → Nincs adatvesztes egyikben sem

TestMigrator_DryRun
    V2 fajlok → dry-run
    → Helyes szamlalo (hany fajlt kell migalni)
    → EGYETLEN fajl sem modosult

TestMigrator_EmptyFile
    Ures V2 fajl (header + 0 block)
    → Migracio sikeres → V3 fajl header-rel + nevvel + 0 block

TestMigrator_FileWithoutMetadata
    V2 fajl OpMetadata NELKUL (regi fajl)
    → Migracio: a nev ures string lesz a header-ben
    → A fajl ettol fuggetlenul V3-ra frissul
    → Nem crash-el
```

### 8.7. Compactor V3 teszt (`v2/compactor_test.go` bovites)

**Cel:** A compaction soran V2 fajlok automatikusan V3-ra frissulnek.

```
TestCompactor_V2toV3Upgrade
    V2 fajl sok torolt entry-vel (magas fragmentacio)
    → Compaction
    → Az eredmeny V3 formatumu fajl
    → OpMetadata eltavolitva a block-okbol
    → Nev a header-ben

TestCompactor_V3toV3_NoDowngrade
    V3 fajl compaction → az eredmeny is V3 marad

TestCompactor_V2Compaction_PreservesAllLiveData
    V2 fajl 500 insert + 200 delete + 100 update
    → Compaction → V3 fajl
    → LoadIndex(): pontosan a vart live entry-k
```

### 8.8. Chronicler V2 adapter teszt (`chronicler_v2_test.go` bovites)

**Cel:** A magasabb szintu chronicler adapter helyesen mukodik V3-mal.

```
TestChroniclerV2_NewSwamp_CreatesV3
    Uj chronicler → Write() → a letrehozott fajl V3 formatumu

TestChroniclerV2_Load_V2File
    V2 fajl betoltese → a swamp nev helyes (block-bol olvassa)

TestChroniclerV2_Load_V3File
    V3 fajl betoltese → a swamp nev helyes (header-bol olvassa)

TestChroniclerV2_WriteToExistingV2
    V2 fajl → uj entry iras → a fajl formatum NEM valtozik (V2 marad)
    (a formatum upgrade CSAK compaction vagy migracio soran tortenik)
```

### 8.9. Stressz teszt (`v2/stress_v3_test.go`)

**Cel:** Nagy terhelest alatt is hibatlanul mukodik.

```
TestStress_V3_ManySmallEntries
    100000 apro entry (10 byte data) → iras → olvasas → mind helyes

TestStress_V3_FewLargeEntries
    100 nagy entry (500KB data) → iras → olvasas → mind helyes

TestStress_V3_ConcurrentReadWrite
    Parhuzamos iras es olvasas (10 goroutine)
    → Nincs panic, nincs data corruption

TestStress_Migration_1000Files
    1000 V2 fajl generálása → parhuzamos migracio → mind V3 → mind helyes
```

### 8.10. Teszteles elvarasai — MINDEN teszt ZOLD kell legyen mielott:

1. **Fejlesztes kozben**: `go test ./app/core/hydra/swamp/chronicler/v2/... -v` ZOLD
2. **Migratornal**: `go test ./app/core/hydra/swamp/chronicler/v2/migrator/... -v` ZOLD
3. **Chronicler adapter**: `go test ./app/core/hydra/swamp/chronicler/... -v` ZOLD
4. **Teljes hydra**: `go test ./app/core/hydra/... -v` ZOLD
5. **A meglevo tesztek EGYIKE SEM TORHET EL** — regresszio tilott

---

## 9. Kockazatok es megoldasok

| Kockazat | Megoldas |
|----------|----------|
| Migracio kozben szerver crash | Temp fajl + atomikus rename → a regi fajl sertetlen marad |
| Nincs eleg disk hely a temp fajloknak | A migrator fajlonkent dolgozik, nem kell dupla hely az egeszhez — csak 1 temp fajl / worker |
| V2 fajlban nincs OpMetadata entry (regi fajl) | Fallback: a nev ures string lesz, a scanner "unknown" swamp-kent jelzi |
| Szerver aktivan ir egy swamp-ot migracio kozben | A migracio a szerveren fut → a swamp lock-ot hasznal → nem utkozik. Alternativa: a migratort allitott szerver mellett futtatni |
| Dupla tomoritest | A teszt 8.2 (TestV3Writer_DataNotDoubleCompressed) explicit ellenorzi |
| Regresszio a meglevo V2 logikaban | A teszt 8.5 (compat tesztek) es a meglevo teszt suite futtatasa biztositja |

---

## 10. Osszefugges a Swamp Explorer-rel

Ez a terv az **elofeltétel** a Swamp Explorer-hez (lasd: `docs/SWAMP_EXPLORER_TERV.md`).

Miutan a V3 formatum es a migracio elkeszul:
- A scanner `ReadSwampName()` fuggvennyel **~100 byte / fajl** olvasassal kepes a nevet kinyerni
- Nincs block dekompresszio → millio fajlnal is masodpercek a teljes scan
- A Swamp Explorer terv a jelenlegi formaban hasznalhato, csak a scanner resze egyszerusodik

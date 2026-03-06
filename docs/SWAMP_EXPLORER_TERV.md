# HydrAIDE Swamp Explorer - Kivitelezesi Terv

## ELOFELTÉTEL

> Ez a terv a `.hyd` fajlformatum V3 bovitesre epit.
> Eloszor a `docs/HYD_FORMAT_V3_TERV.md` szerinti munkakat kell elvegezni.
> A V3 formatumban a swamp neve kozvetlenul a header utan van tarolva,
> igy a scanner block dekompresszio nelkul, ~100 byte / fajl olvasassal
> kepes a nevet kinyerni.

## 1. Osszefoglalo

**Cel:** Egy admin rendszer, amely kepes a HydrAIDE fajlrendszerben levo osszes swamp felterkepezesere a `.hyd` fajlok header+nev olvasasaval (NEM a teljes adat beolvasasaval), es ezeket kereshetove, listazhatova teszi Sanctuary / Realm / Swamp hierarchia szerint.

**Erintett komponensek:**
- `proto/hydraide.proto` — uj gRPC vegpontok
- `app/server/gateway/` — uj handler-ek
- `app/server/explorer/` — **uj package**: fajlrendszer scanner + in-memory index
- `app/hydraidectl/cmd/` — uj `explore` parancs
- `generated/hydraidepbgo/` — ujrageneralt proto stub-ok

---

## 2. Meglevo allapot elemzese

### Amit mar van:
- `.hyd` fajlok tartalmaznak `OpMetadata` entry-t a swamp nevevel (`__swamp_meta__` kulcs)
- `FileHeader` (64 byte) tartalmazza: `CreatedAt`, `ModifiedAt`, `EntryCount`, `BlockCount`
- `v2.NewFileReader()` kepes megnyitni es olvasni a header-t + entry-ket
- `hydraidectl` mar rendelkezik hasonlo admin parancsokkal (observe, stats, size)
- A fajlrendszer struktura determinisztikus: `<data_root>/<islandID>/<hash_subfolder>/<swamp_hash>.hyd`
- `IslandID`-k: 1-1000, hash subfolder-ek: 1 szint melyseg

### Amit ujra fel lehet hasznalni:
- `v2.FileReader` — header + metadata olvasas (de kell egy "csak metadata" mod)
- `v2.FileHeader` — mar tartalmazza a fajl alap statisztikait
- `app/name.Load()` — swamp nev visszaallitasa stringbol
- `hydraidectl` cobra pattern — uj parancs hozzaadasa
- Telemetry/observe gRPC stream minta — hasonlo architektura

### Ami hianyzik:
- Fajlrendszer scanner, ami vegigjar minden island/subfolder-t
- In-memory index a scan eredmenyeinek tarolasara
- gRPC vegpontok a lekerdezeshez
- Gyors metadata-only olvasas (csak header + elso metadata entry, NEM az osszes treasure)

---

## 3. Architektura

### 3.1. Uj szerver-oldali package: `app/server/explorer/`

```
app/server/explorer/
    explorer.go      — Explorer interface + implementacio
    scanner.go       — parhuzamos fajlrendszer scanner
    index.go         — in-memory hierarchikus index
```

**Explorer interface:**

```go
type Explorer interface {
    // Scan elinditja a fajlrendszer scannelest hatterben.
    // Ha mar fut egy scan, hibat ad vissza.
    Scan() error

    // GetScanStatus visszaadja a scan allapotat (idle/running/done + statisztikak).
    GetScanStatus() *ScanStatus

    // ListSanctuaries visszaadja az osszes Sanctuary-t darabszammal es merettel.
    ListSanctuaries() []*SanctuaryInfo

    // ListRealms visszaadja egy Sanctuary osszes Realm-jet.
    ListRealms(sanctuary string) []*RealmInfo

    // ListSwamps visszaadja a swampokat szurovel es lapozoval.
    ListSwamps(filter *SwampFilter) *SwampListResult

    // GetSwampDetail visszaadja egy konkret swamp reszletes fajl-informacioit.
    GetSwampDetail(sanctuary, realm, swamp string) (*SwampDetail, error)

    // GetSize visszaadja a meretet Sanctuary, Sanctuary/Realm, vagy Sanctuary/Realm/Swamp szinten.
    GetSize(sanctuary, realm, swamp string) (*SizeInfo, error)
}
```

**Adatstrukturak:**

```go
type ScanStatus struct {
    State         string    // "idle", "running", "done", "error"
    StartedAt     time.Time
    FinishedAt    time.Time
    TotalFiles    int64     // hany .hyd fajlt talalt
    ScannedFiles  int64     // hany .hyd fajlt dolgozott fel eddig
    ErrorCount    int64     // hany fajlnal volt hiba
    Duration      time.Duration
}

type SanctuaryInfo struct {
    Name        string
    RealmCount  int64
    SwampCount  int64
    TotalSize   int64   // byte-ban
}

type RealmInfo struct {
    Sanctuary   string
    Name        string
    SwampCount  int64
    TotalSize   int64
}

type SwampDetail struct {
    Sanctuary   string
    Realm       string
    Swamp       string
    FilePath    string    // teljes elesi ut a .hyd fajlhoz
    FileSize    int64     // fajlmeret byte-ban
    CreatedAt   time.Time // a .hyd header-bol
    ModifiedAt  time.Time // a .hyd header-bol
    EntryCount  uint64    // elso ertekek (live entry-k szama)
    BlockCount  uint64    // block-ok szama
    IslandID    string    // melyik island-on van
}

type SwampFilter struct {
    Sanctuary  string  // opcionalis szuro
    Realm      string  // opcionalis szuro
    Swamp      string  // opcionalis szuro (prefix match)
    Offset     int64   // lapozas
    Limit      int64   // oldalmerert (default 100)
}

type SwampListResult struct {
    Swamps     []*SwampDetail
    Total      int64   // osszes talalat (lapozas nelkul)
    Offset     int64
    Limit      int64
}

type SizeInfo struct {
    Sanctuary   string
    Realm       string  // ures ha sanctuary szintu
    Swamp       string  // ures ha realm szintu
    TotalSize   int64
    FileCount   int64
}
```

### 3.2. Scanner mukodese (scanner.go)

```
1. Vegigiteralunk az <data_root>/ mappan
2. Minden <islandID> (1-1000) mappahoz:
   a. Vegigjarjuk a hash subfolder-eket
   b. Minden subfolder-ben keressuk a .hyd fajlokat
3. Minden .hyd fajlhoz:
   a. os.Stat() → fajlmeret
   b. Megnyitjuk, olvassuk a 64 byte FileHeader-t
   c. Olvassuk az elso BlockHeader-t (16 byte) + az elso compressed block-ot
      (a swamp nev OpMetadata entry-kent altalaban itt van)
   d. Ha megvan a nev → name.Load(swampName) → Sanctuary/Realm/Swamp
   e. Bezarjuk a fajlt
   Osszesen: 64B header + 16B block header + elso block (~nehany szaz byte - par KB)
   Tipikusan ~1-5KB olvasas / fajl (NEM a teljes adat!)
4. Az eredmenyt berakjuk az in-memory index-be
```

**Parhuzamossag:**
- Worker pool: `runtime.NumCPU()` goroutine (de panichandler.SafeGo-val!)
- Minden island kulon task → channel-en keresztul adjuk a worker-eknek
- A scan allapotot atomikusan frissitjuk (ScannedFiles counter)

**Fontos:** A scanner NEM olvassa be a treasure adatokat! Csak:
- 64 byte FileHeader
- Block-okat addig amig az `OpMetadata` / `__swamp_meta__` entry megvan
- Ez altalaban az elso 1-2 block (max ~32KB olvasas / fajl)

### 3.3. In-memory index (index.go)

```go
type index struct {
    mu          sync.RWMutex
    sanctuaries map[string]*sanctuaryNode  // sanctuary name → node
}

type sanctuaryNode struct {
    realms    map[string]*realmNode
    totalSize int64
}

type realmNode struct {
    swamps    map[string]*SwampDetail
    totalSize int64
}
```

- A scan eredmenyet ide gyujtjuk
- Thread-safe (RWMutex)
- Uj scan elott az index torlodik es ujra feltoltodik
- Nincs perzisztencia — a scan on-demand tortenik

### 3.4. Proto vegpontok

Uj section a `hydraide.proto`-ban:

```protobuf
// ============ EXPLORER ENDPOINTS ============

// ScanSwamps elinditja a fajlrendszer scannelest a swampok felterkepezesere.
// A scan hatterben fut, az allapotot a GetScanStatus-szal lehet lekerdezni.
rpc ScanSwamps(ScanSwampsRequest) returns (ScanSwampsResponse) {}

// GetScanStatus visszaadja a scan allapotat.
rpc GetScanStatus(GetScanStatusRequest) returns (GetScanStatusResponse) {}

// ListSanctuaries visszaadja az osszes Sanctuary-t darabszammal es merettel.
rpc ListSanctuaries(ListSanctuariesRequest) returns (ListSanctuariesResponse) {}

// ListRealms visszaadja egy Sanctuary osszes Realm-jet.
rpc ListRealms(ListRealmsRequest) returns (ListRealmsResponse) {}

// ListSwamps visszaadja a swampokat szurovel es lapozoval.
rpc ListSwamps(ListSwampsRequest) returns (ListSwampsResponse) {}

// GetSwampDetail visszaadja egy konkret swamp reszletes fajl-informacioit.
rpc GetSwampDetail(GetSwampDetailRequest) returns (GetSwampDetailResponse) {}

// GetSize visszaadja az osszesitett meretet Sanctuary, Realm vagy Swamp szinten.
rpc GetSize(GetSizeRequest) returns (GetSizeResponse) {}
```

**Uj message-ek:**

```protobuf
// --- Scan ---
message ScanSwampsRequest {}
message ScanSwampsResponse {
    string status = 1;       // "started" vagy "already_running"
    string message = 2;
}

message GetScanStatusRequest {}
message GetScanStatusResponse {
    string state = 1;              // "idle", "running", "done", "error"
    google.protobuf.Timestamp started_at = 2;
    google.protobuf.Timestamp finished_at = 3;
    int64 total_files = 4;
    int64 scanned_files = 5;
    int64 error_count = 6;
    int64 duration_ms = 7;
}

// --- Sanctuaries ---
message ListSanctuariesRequest {}
message ListSanctuariesResponse {
    repeated SanctuaryInfo sanctuaries = 1;
}
message SanctuaryInfo {
    string name = 1;
    int64 realm_count = 2;
    int64 swamp_count = 3;
    int64 total_size = 4;
}

// --- Realms ---
message ListRealmsRequest {
    string sanctuary = 1;
}
message ListRealmsResponse {
    repeated RealmInfo realms = 1;
}
message RealmInfo {
    string sanctuary = 1;
    string name = 2;
    int64 swamp_count = 3;
    int64 total_size = 4;
}

// --- Swamps ---
message ListSwampsRequest {
    string sanctuary = 1;    // koteleto
    string realm = 2;        // opcionalis — ha ures, az osszes realm-bol listaz
    string swamp_prefix = 3; // opcionalis — prefix szures a swamp nevre
    int64 offset = 4;
    int64 limit = 5;         // default 100, max 1000
}
message ListSwampsResponse {
    repeated SwampDetailInfo swamps = 1;
    int64 total = 2;
    int64 offset = 3;
    int64 limit = 4;
}

// --- Swamp Detail ---
message GetSwampDetailRequest {
    string sanctuary = 1;
    string realm = 2;
    string swamp = 3;
}
message GetSwampDetailResponse {
    SwampDetailInfo swamp = 1;
}

message SwampDetailInfo {
    string sanctuary = 1;
    string realm = 2;
    string swamp = 3;
    string file_path = 4;
    int64 file_size = 5;
    google.protobuf.Timestamp created_at = 6;
    google.protobuf.Timestamp modified_at = 7;
    uint64 entry_count = 8;
    uint64 block_count = 9;
    string island_id = 10;
}

// --- Size ---
message GetSizeRequest {
    string sanctuary = 1;    // koteleto
    string realm = 2;        // opcionalis
    string swamp = 3;        // opcionalis
}
message GetSizeResponse {
    int64 total_size = 1;
    int64 file_count = 2;
    string sanctuary = 3;
    string realm = 4;
    string swamp = 5;
}
```

### 3.5. Gateway handler-ek

Az `app/server/gateway/gateway.go`-ban uj metodusok:
- `ScanSwamps()` — meghivja `explorer.Scan()`-t
- `GetScanStatus()` — meghivja `explorer.GetScanStatus()`-t
- `ListSanctuaries()` — meghivja `explorer.ListSanctuaries()`-t
- `ListRealms()` — meghivja `explorer.ListRealms()`-t
- `ListSwamps()` — meghivja `explorer.ListSwamps()`-t
- `GetSwampDetail()` — meghivja `explorer.GetSwampDetail()`-t
- `GetSize()` — meghivja `explorer.GetSize()`-t

Az Explorer peldany a `Gateway` struct-ba kerul (mint a `TelemetryCollector`).

### 3.6. hydraidectl `explore` parancs

```
hydraidectl explore --instance <nev>
```

**Mukodes:**
1. Csatlakozik a szerverre (ugyan ugy mint az observe parancs)
2. Eloszor meghivja a `ScanSwamps` RPC-t → varja a `GetScanStatus`-t amig "done"
3. Kilistazza a Sanctuary-kat tablazatban
4. A user valaszthat → Realm-ek → Swamp-ok (interaktiv TUI, mint az observe)

**Alternativ egyszerubb mod:**
```
hydraidectl explore --instance prod --scan                    # scan inditasa
hydraidectl explore --instance prod --list                    # sanctuaries listazasa
hydraidectl explore --instance prod --list --sanctuary users  # realms listazasa
hydraidectl explore --instance prod --list --sanctuary users --realm profiles  # swamps
hydraidectl explore --instance prod --detail --sanctuary users --realm profiles --swamp user123
hydraidectl explore --instance prod --size --sanctuary users  # osszesitett meret
```

---

## 4. Fazisokra bontott megvalositas

### 1. Fazis: Explorer core (scanner + index)
- [ ] `app/server/explorer/explorer.go` — Explorer interface + constructor
- [ ] `app/server/explorer/index.go` — in-memory hierarchikus index
- [ ] `app/server/explorer/scanner.go` — parhuzamos fajlrendszer scanner
- [ ] Gyors metadata-only olvasas a `v2.FileReader`-bol (csak header + metadata entry)
- [ ] Unit testek a scanner-hez es index-hez

**Fazis statusz:** ⏳

### 2. Fazis: Proto + Gateway
- [ ] `proto/hydraide.proto` boszites az uj Explorer vegpontokkal
- [ ] `make proto-go` ujrageneralas
- [ ] `app/server/gateway/` — uj handler metodusok
- [ ] Explorer peldany integracio a `server.go`-ban (Gateway struct-ba)

**Fazis statusz:** ⏳

### 3. Fazis: hydraidectl explore parancs
- [ ] `app/hydraidectl/cmd/explore.go` — cobra parancs flag-ekkel
- [ ] gRPC kliens hivasok a scan/list/detail/size vegpontokhoz
- [ ] Tablazatos kimenet (lipgloss formatazas, mint a tobbi parancsnal)

**Fazis statusz:** ⏳

### 4. Fazis: Tesztek + finomhangolas
- [ ] E2E teszt: scan → list → detail flow
- [ ] Teljesitmeny teszt nagy szamu fajllal
- [ ] Edge case-ek: ures adatbazis, korrupt .hyd fajlok, V1 fajlok kezelese

**Fazis statusz:** ⏳

---

## 5. Fontos technikai dontesek

### Miert a szerveren es nem a hydraidectl-ben?
- A szerver fer hozza kozvetlenul a fajlrendszerhez
- A scan eredmenye memoriaban marad es tobbszor lekerdezhetoi kulonbozo kliensekbol
- A frontend (pl. web dashboard) is hasznalhatja ugyan ezeket a gRPC vegpontokat
- A hydraidectl csak egy kliens, ami ezeket a vegpontokat hivja

### Miert on-demand scan es nem automatikus?
- Milliok .hyd fajl eseten a scan percekig tarthat
- Nem akarjuk a szerver indulaskor lassitani
- A user donti el, mikor akar friss adatokat
- A scan allapota lekerdezhetoi → progressbar a frontenden

### V1 fajlok kezelese
- V1 swamp-ok mappaval + `meta` fajllal rendelkeznek (nem .hyd)
- A scanner felismeri ezeket is: ha egy mappa `meta` fajlt tartalmaz, abbol olvassa ki a swamp nevet
- Igy vegyes V1/V2 kornyezetben is mukodik

### Teljesitmeny becslese
- 1 millio .hyd fajl, fajlonkent ~1-5KB olvasas = ~1-5GB I/O
  (64B FileHeader + 16B BlockHeader + elso compressed block)
- 8 CPU maggal parhuzamositva, SSD-n: ~30s - 2 perc
- Az in-memory index ~1M swamp eseten kb. 200-500MB RAM

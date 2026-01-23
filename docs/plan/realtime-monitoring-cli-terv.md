# HydrAIDE Realtime Monitoring CLI - Kivitelezési Terv

**Létrehozva:** 2026-01-23  
**Státusz:** ⏳ Terv jóváhagyásra vár

---

## 1. Összefoglaló

### Cél
Egy új `hydraidectl observe` parancs létrehozása, amely **realtime TUI dashboardot** biztosít a HydrAIDE szerver összes gRPC hívásának, hibájának és kliens aktivitásának megfigyelésére.

### Érintett komponensek
- `app/server/server/server.go` - Interceptor bővítés
- `app/server/observer/` - Új telemetria modul
- `app/hydraidectl/cmd/observe.go` - Új CLI parancs
- `proto/hydraide.proto` - Új streaming endpoint a telemetriához
- Új függőség: `github.com/charmbracelet/bubbletea` + `bubbles` + `lipgloss` (TUI)

---

## 2. Meglévő állapot elemzése

### ✅ Ami már létezik és felhasználható:
1. **gRPC Interceptor** (`server.go:122-180`)
   - Már kinyeri a client IP-t
   - Logolja a hibákat (ha `GRPC_SERVER_ERROR_LOGGING=true`)
   - **Bővítendő:** telemetria adatok küldése

2. **Observer package** (`app/server/observer/observer.go`)
   - Követi a futó folyamatokat (StartProcess/EndProcess)
   - Van memória és CPU monitoring
   - **Bővítendő:** gRPC call telemetria

3. **Cobra CLI struktúra** (`app/hydraidectl/cmd/`)
   - Jól strukturált parancsok
   - Root command készen áll új subcommand fogadására

### ❌ Ami hiányzik:
1. Nincs gRPC streaming endpoint a telemetria lekérdezéshez
2. Nincs TUI könyvtár a projektben
3. Nincs call-level metrika gyűjtés (swamp név, key, művelet típus)
4. Nincs kliens session tracking

---

## 3. Tervezett Architektúra

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           HydrAIDE Server                                │
│                                                                          │
│  ┌──────────────┐    ┌──────────────────┐    ┌────────────────────────┐ │
│  │    gRPC      │───▶│   Interceptor    │───▶│   Telemetry Collector  │ │
│  │   Requests   │    │   (bővített)     │    │   + Error Details      │ │
│  └──────────────┘    └──────────────────┘    └───────────┬────────────┘ │
│                                                           │              │
│                                              ┌────────────▼───────────┐ │
│                                              │  Time-Based Storage    │ │
│                                              │  (30 perc history)     │ │
│                                              │  - Ring Buffer (calls) │ │
│                                              │  - Error Store (full)  │ │
│                                              └────────────┬───────────┘ │
│                                                           │              │
│  ┌────────────────────────────────────────────────────────┴───────────┐ │
│  │  gRPC Streaming Endpoints:                                          │ │
│  │  - SubscribeToTelemetry (realtime stream)                          │ │
│  │  - GetTelemetryHistory (replay X minutes)                          │ │
│  │  - GetErrorDetails (full error with stack trace)                   │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC Stream / Request
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        hydraidectl observe                               │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                     Bubbletea TUI Dashboard                         │ │
│  │                                                                      │ │
│  │  [1] Live  [2] Replay  [3] Errors  [4] Stats      [H]elp  [Q]uit   │ │
│  │  ══════════════════════════════════════════════════════════════════ │ │
│  │                                                                      │ │
│  │  ┌──────────────────────────────────────────────────────────────┐   │ │
│  │  │ 📊 Live Calls                                    [P]ause     │   │ │
│  │  │ ──────────────────────────────────────────────────────────── │   │ │
│  │  │ 14:23:01.234 │ GET    │ user/sessions/abc │ 2.3ms │ ✓       │   │ │
│  │  │ 14:23:01.456 │ SET    │ cache/products/x  │ 1.1ms │ ✓       │   │ │
│  │  │▶14:23:01.789 │ GET    │ auth/tokens/xyz   │ 5.2ms │ ✗ ERROR │   │ │
│  │  │ 14:23:02.012 │ DELETE │ temp/uploads/file │ 0.8ms │ ✓       │   │ │
│  │  └──────────────────────────────────────────────────────────────┘   │ │
│  │                                                                      │ │
│  │  ┌──────────────────────────────────────────────────────────────┐   │ │
│  │  │ 🔍 Error Details (Press ENTER to expand selected error)      │   │ │
│  │  │ ──────────────────────────────────────────────────────────── │   │ │
│  │  │ Time:     14:23:01.789                                        │   │ │
│  │  │ Method:   Get                                                 │   │ │
│  │  │ Swamp:    auth/tokens/xyz                                     │   │ │
│  │  │ Keys:     ["user_abc123"]                                     │   │ │
│  │  │ Error:    FailedPrecondition: decompression failed            │   │ │
│  │  │ Client:   192.168.1.50                                        │   │ │
│  │  │ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─  │   │ │
│  │  │ Stack Trace:                                                  │   │ │
│  │  │   chronicler/v2/engine.go:234 - Decompress()                 │   │ │
│  │  │   swamp/treasure/guard.go:89 - LoadTreasure()                │   │ │
│  │  │   gateway/gateway.go:456 - Get()                             │   │ │
│  │  └──────────────────────────────────────────────────────────────┘   │ │
│  │                                                                      │ │
│  │  ┌───────────────────┐ ┌─────────────────────────────────────────┐  │ │
│  │  │ 📈 Stats (5m/15m) │ │ 🔴 Recent Errors (last 30 min)          │  │ │
│  │  │ Calls: 1234/4567  │ │ [3x] DecompressError: auth/tokens       │  │ │
│  │  │ Errors: 12/45     │ │ [1x] InvalidArgument: Set (missing key) │  │ │
│  │  │ Clients: 8 active │ │ [1x] NotFound: user/sessions            │  │ │
│  │  │ Avg: 2.3ms        │ │                                         │  │ │
│  │  └───────────────────┘ └─────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ═══════════════════════════════════════════════════════════════════════ │
│  [R] Replay Mode │ From: 14:20:00  To: 14:25:00  │ [▶] Play [⏸] Pause   │
│  Speed: [1x] [2x] [4x]   Filter: auth/*   Errors only: [x]              │
│  ═══════════════════════════════════════════════════════════════════════ │
└─────────────────────────────────────────────────────────────────────────┘
```

### Fő Funkciók:

1. **Live Mode** - Realtime stream, pausolható, scrollozható
2. **Replay Mode** - Visszajátszás az elmúlt 30 percből (konfigurálható)
3. **Error Details** - Kibontható error panel teljes stack trace-szel
4. **Filter** - Swamp pattern, method típus, errors only mód
5. **Debug Support** - Pontosan látható mi történik egy adott hívásnál (pl. bejelentkezésnél)

---

## 4. Fázisokra Bontott Megvalósítás

### 1. Fázis: Telemetry Collector és Time-Based Storage (Server oldal)

**Cél:** Központi telemetria gyűjtő létrehozása időalapú tárolással és replay képességgel.

- [x] Új package létrehozása: `app/server/telemetry/telemetry.go`
  - `TelemetryEvent` struktúra:
    ```go
    type TelemetryEvent struct {
        ID          string            // Unique event ID
        Timestamp   time.Time         // When the call happened
        Method      string            // gRPC method name (Get, Set, Delete, etc.)
        SwampName   string            // Full swamp path (sanctuary/realm/swamp)
        Keys        []string          // Affected keys
        DurationMs  int64             // Call duration in milliseconds
        Success     bool              // Was the call successful
        ErrorCode   string            // gRPC error code (if error)
        ErrorMsg    string            // Error message (if error)
        StackTrace  string            // Stack trace for errors (optional)
        ClientIP    string            // Client IP address
        RequestSize int64             // Request payload size in bytes
        ResponseSize int64            // Response payload size in bytes
    }
    ```
  - `TelemetryCollector` interface és implementáció
  - Thread-safe **time-based ring buffer** (30 perc history, ~100,000 event)
  - Subscriber pattern a realtime stream kliensekhez
  - **Time-range query** támogatás a replay funkcióhoz

- [x] Új package: `app/server/telemetry/errorstore.go`
  - Külön error store a részletes hibainformációkhoz
  - Stack trace tárolás (runtime.Stack() alapú)
  - Error aggregáció (hasonló hibák csoportosítása)
  - 30 perces retention (konfigurálható)

- [x] Új package: `app/server/telemetry/stats.go` (-> clients.go)
  - Aggregált statisztikák (calls/min, errors/min, unique clients)
  - Sliding window számítások (1 perc, 5 perc, 15 perc)
  - Top N swamp by calls/errors

- [x] Unit tesztek a telemetry package-hez

**Fázis státusz:** ✅ Kész

---

### 2. Fázis: Interceptor Bővítés (Server oldal)

**Cél:** A meglévő interceptor bővítése telemetria küldéssel.

- [x] `server.go` interceptor módosítása:
  - Request/response részletek kinyerése (swamp név, művelet típus)
  - Duration mérés (handler előtt/után)
  - Telemetria event létrehozása és küldése a collector-nak

- [x] Gateway módosítások:
  - TelemetryCollector mező hozzáadása
  - Import frissítése

- [x] Helper függvények:
  - `extractMethodName()` - gRPC metódus név kinyerése
  - `extractSwampName()` - Swamp név kinyerése a requestből
  - `extractKeys()` - Érintett kulcsok kinyerése
  - `formatSwampPath()` - IslandID + SwampName formázása

**Fázis státusz:** ✅ Kész

---

### 3. Fázis: gRPC Endpoints (Proto + Server)

**Cél:** gRPC endpointok a realtime stream, history replay és error details lekérdezéshez.

- [ ] Proto bővítés (`hydraide.proto`):
  ```protobuf
  // ============ TELEMETRY ENDPOINTS ============
  
  // Real-time telemetry stream
  rpc SubscribeToTelemetry(TelemetrySubscribeRequest) returns (stream TelemetryEvent) {}
  
  // Get historical events for replay (last X minutes)
  rpc GetTelemetryHistory(TelemetryHistoryRequest) returns (TelemetryHistoryResponse) {}
  
  // Get detailed error information with stack trace
  rpc GetErrorDetails(ErrorDetailsRequest) returns (ErrorDetailsResponse) {}
  
  // Get aggregated statistics
  rpc GetTelemetryStats(TelemetryStatsRequest) returns (TelemetryStatsResponse) {}
  
  // ============ MESSAGES ============
  
  message TelemetrySubscribeRequest {
    bool include_successful_calls = 1;  // Include successful calls (default: true)
    bool errors_only = 2;               // Only stream errors
    repeated string filter_methods = 3; // Filter by method names (empty = all)
    string filter_swamp_pattern = 4;    // Swamp name pattern filter (e.g., "auth/*")
  }
  
  message TelemetryEvent {
    string id = 1;
    google.protobuf.Timestamp timestamp = 2;
    string method = 3;
    string swamp_name = 4;
    repeated string keys = 5;
    int64 duration_ms = 6;
    bool success = 7;
    string error_code = 8;
    string error_message = 9;
    string client_ip = 10;
    int64 request_size = 11;
    int64 response_size = 12;
    bool has_stack_trace = 13;  // True if detailed error available
  }
  
  message TelemetryHistoryRequest {
    google.protobuf.Timestamp from_time = 1;  // Start of time range
    google.protobuf.Timestamp to_time = 2;    // End of time range
    bool errors_only = 3;
    string filter_swamp_pattern = 4;
    repeated string filter_methods = 5;
    int32 limit = 6;  // Max events to return (default: 1000)
  }
  
  message TelemetryHistoryResponse {
    repeated TelemetryEvent events = 1;
    int32 total_count = 2;      // Total matching events
    bool has_more = 3;          // More events available
  }
  
  message ErrorDetailsRequest {
    string event_id = 1;  // The TelemetryEvent ID
  }
  
  message ErrorDetailsResponse {
    TelemetryEvent event = 1;
    string stack_trace = 2;           // Full stack trace
    string error_category = 3;        // Categorized error type
    map<string, string> context = 4;  // Additional context (request details, etc.)
  }
  
  message TelemetryStatsRequest {
    int32 window_minutes = 1;  // Stats window (1, 5, 15 minutes)
  }
  
  message TelemetryStatsResponse {
    int64 total_calls = 1;
    int64 error_count = 2;
    double error_rate = 3;
    double avg_duration_ms = 4;
    int32 active_clients = 5;
    repeated SwampStats top_swamps = 6;
    repeated ErrorSummary top_errors = 7;
  }
  
  message SwampStats {
    string swamp_name = 1;
    int64 call_count = 2;
    int64 error_count = 3;
    double avg_duration_ms = 4;
  }
  
  message ErrorSummary {
    string error_code = 1;
    string error_message = 2;
    int64 count = 3;
    string last_swamp = 4;
    google.protobuf.Timestamp last_occurrence = 5;
  }
  ```

- [ ] Proto újragenerálás (`make proto`)
- [ ] Gateway implementáció:
  - `SubscribeToTelemetry` - realtime stream handler
  - `GetTelemetryHistory` - history query a replay-hez
  - `GetErrorDetails` - részletes error lekérés stack trace-szel
  - `GetTelemetryStats` - aggregált statisztikák

**Fázis státusz:** ✅ Kész

---

### 4. Fázis: Bubbletea TUI Dashboard (CLI oldal)

**Cél:** Interaktív terminál UI a hydraidectl-ben, replay móddal és kibontható error panellel.

- [x] Függőségek hozzáadása:
  ```
  github.com/charmbracelet/bubbletea
  github.com/charmbracelet/bubbles
  github.com/charmbracelet/lipgloss
  ```

- [x] Új command: `app/hydraidectl/cmd/observe.go`
  - Flags:
    - `--errors-only` - Csak hibák mutatása
    - `--filter` - Swamp pattern filter (pl. `auth/*`)
    - `--simple` - Egyszerű szöveges kimenet TUI helyett
    - `--stats` - Csak statisztikák mutatása
  - gRPC kliens a telemetria endpointokhoz
  
- [x] TUI komponensek (`app/hydraidectl/cmd/observe/`):
  - `model.go` - Bubbletea model (state management, view rendering)
  - `styles.go` - Lipgloss stílusok
  - `styles.go` - Lipgloss stílusok
  - `keys.go` - Keyboard bindings

- [ ] **Live Mode funkciók:**
  - Realtime stream megjelenítése
  - Pausolás [P] - stream megáll, de puffereli az eseményeket
  - Scrollozás [↑↓] a call listában
  - Szűrés [/] - gyors filter input

- [ ] **Replay Mode funkciók:**
  - [R] - Replay mód be/ki kapcsolás
  - Időablak választás (From/To)
  - Lejátszás sebessége: [1x] [2x] [4x] [8x]
  - [Space] - Play/Pause
  - [←→] - Léptetés előre/hátra

- [x] **Error Details funkciók:**
  - [Enter] - Kiválasztott error kibontása
  - Stack trace megjelenítése (alapok)
  - Request/Response részletek
  - [Esc] - Bezárás

- [x] **Általános billentyűk:**
  - [1] Live mode
  - [2] Errors panel
  - [3] Stats panel
  - [?/H] Help
  - [Q] Kilépés
  - [C] Clear screen
  - [P] Pause/Resume
  - [E] Errors only filter

**Fázis státusz:** ✅ Kész

---

### 5. Fázis: Kliens Tracking és Statisztikák

**Cél:** Aktív kliensek és részletes statisztikák megjelenítése.

- [x] Client session tracking a telemetry collector-ban:
  - Unique client IP-k számlálása
  - Utolsó aktivitás időbélyeg per kliens
  - Calls per client statisztika

- [x] Aggregált metrikák:
  - Total calls (window ablak)
  - Error rate %
  - Avg response time
  - Top N legaktívabb swamp

- [x] TUI Stats panel

**Fázis státusz:** ✅ Kész

---

### 6. Fázis: Error Kategorizálás és Részletek

**Cél:** Részletes hibainformációk megjelenítése.

- [x] Error típusok kategorizálása:
  - Compression/Decompression errors
  - Validation errors (InvalidArgument)
  - Permission errors
  - Timeout errors
  - Internal errors

- [x] Error részletek a TUI-ban:
  - Error code és message
  - Érintett swamp/key
  - Időbélyeg és előfordulás száma

- [x] Error aggregáció (hasonló hibák csoportosítása)

**Fázis státusz:** ✅ Kész

---

### 7. Fázis: Tesztelés és Finomhangolás

**Cél:** Teljes körű tesztelés és optimalizálás.

- [ ] Integration tesztek a telemetria stream-re
- [ ] Performance teszt (nagy terhelés mellett is működjön)
- [ ] Memory leak ellenőrzés (hosszú futás)
- [ ] Edge case-ek kezelése:
  - Szerver leáll közben
  - Hálózati hiba
  - Túl sok esemény (throttling)

**Fázis státusz:** ⏳ Várakozik

---

### 8. Fázis: Dokumentáció

**Cél:** Teljes dokumentáció a fejlesztőknek és felhasználóknak.

- [x] CLI dokumentáció (`docs/hydraidectl/hydraidectl-user-manual.md`):
  - observe és telemetry szekciók hozzáadva
  - Használat és flag-ek
  - Billentyűparancsok
  - Példák és use case-ek

- [x] root.go Long description frissítve:
  - observe és telemetry parancsok hozzáadva

**Fázis státusz:** ✅ Kész

---

### 9. Fázis: CHANGELOG

- [ ] `docs/changelogs/2026-01-23.md` frissítése
- [ ] Feature dokumentálás a changelog-ban

**Fázis státusz:** ⏳ Várakozik

---

## 5. Technikai Döntések és Indoklások

### Miért Bubbletea?
- Go natív, modern TUI framework
- Elm-szerű architektúra (tiszta, tesztelhető)
- Aktív fejlesztés, jó dokumentáció
- Lipgloss-szal kombinálva szép UI

### Miért Ring Buffer?
- Fix memóriahasználat (nem nő korlátlanul)
- O(1) insert/read
- Régi események automatikus eldobása

### Miért gRPC Streaming?
- Már használjuk a projektben
- Beépített backpressure
- TLS/mTLS támogatás (biztonságos)
- Bi-directional lehetőség a jövőben (filter változtatás)

### Miért nem külső szolgáltatás (Grafana, etc.)?
- Zero dependency deployment
- Nincs szükség extra infrastruktúrára
- Azonnali használat a terminalban
- A cél: gyors debug, nem long-term monitoring

---

## 6. Becsült Időigény

| Fázis | Becsült idő |
|-------|-------------|
| 1. Telemetry Collector | 2-3 óra |
| 2. Interceptor bővítés | 1-2 óra |
| 3. gRPC Streaming | 2-3 óra |
| 4. TUI Dashboard | 4-6 óra |
| 5. Kliens tracking | 2-3 óra |
| 6. Error kategorizálás | 2-3 óra |
| 7. Tesztelés | 2-3 óra |
| 8. Dokumentáció | 1-2 óra |
| 9. CHANGELOG | 0.5 óra |
| **Összesen** | **17-26 óra** |

---

## 7. Alternatív Megközelítések (elvetett)

### ❌ Webes Dashboard
- **Elvetés oka:** Extra függőségek (HTTP server, frontend), túl komplex erre a célra

### ❌ Log file tailing
- **Elvetés oka:** Nem realtime, nehéz parse-olni, nem interaktív

### ❌ Prometheus + Grafana
- **Elvetés oka:** Túl nagy overhead dev/debug célra, extra infrastruktúra

---

## 8. Kockázatok és Mitigáció

| Kockázat | Valószínűség | Hatás | Mitigáció |
|----------|--------------|-------|-----------|
| Performance impact a telemetriától | Közepes | Közepes | Ring buffer, sampling opció |
| TUI kompatibilitási problémák | Alacsony | Alacsony | Bubbletea jól tesztelt |
| Memory leak hosszú futásnál | Alacsony | Magas | Fixed size buffer, tesztelés |

---

## 9. Jóváhagyás

**Péter, kérlek nézd át a tervet és jelezd:**
1. ✅ Jóváhagyod a tervet és kezdhetjük az 1. fázist?
2. ❓ Van kérdésed vagy módosítási javaslatod?
3. ❌ Más irányt szeretnél?

**Várom a visszajelzésedet a folytatáshoz!**

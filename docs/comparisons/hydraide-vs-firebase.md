## HydrAIDE vs Firebase

Firebase is a **cloud-tied**, **proprietary**, Google-managed platform optimized for mobile backends and real-time 
sync, but it comes at a hidden cost: vendor lock-in, CLI clutter, and surprising bills once you scale.

HydrAIDE, in contrast, is a **logic-native**, **self-hostable**, and **file-free** runtime for reactive systems,
with **type safety**, **per-key locking**, and **zero orchestration** needed.

Where Firebase starts "easy" but ends in cost, complexity, and lock-in, HydrAIDE starts simple and stays yours.

---

## Feature comparison

| Feature            | HydrAIDE                            | Firebase Realtime DB / Firestore        |
| ------------------ |-------------------------------------| --------------------------------------- |
| 📦 Storage model   | ✅ Typed structs via Go              | ❌ JSON blobs with client-side shaping   |
| 🔐 Locking         | ✅ Native per-key `Lock()/Unlock()`  | ❌ No real locking, race-prone logic     |
| 🔄 Reactivity      | ✅ Built-in `Subscribe()` per event  | ✅ Realtime sync, but webhooks limited   |
| 🧠 Type safety     | ✅ GOB-encoded, binary Go structs    | ❌ Client must interpret JSON structure  |
| 🧹 TTL / Cleanup   | ✅ `expireAt` + `ShiftExpired()`     | ⚠️ TTL via background job or rules      |
| 🚦 Sharding        | ✅ Hash-based, deterministic         | ❌ Opaque, controlled by Firebase infra  |
| 🧰 Deployment      | ✅ Embedded, binary, or Docker       | ❌ CLI required, tied to Google Cloud    |
| 💸 Cost scaling    | ✅ Flat infra, self-hosted or Docker | ❌ Grows exponentially with reads/writes |
| 📦 Offline support | ✅ File-free, memory-first Swamps    | ⚠️ Some caching, but online-first model |
| 🧗 Learning curve  | 🟢 Zero-to-Hero in 1 day            | 🟡 Firebase rules + CLI + pricing traps |

---

## Use case comparison

| Use Case                  | HydrAIDE                            | Firebase                                 |
| ------------------------- | ----------------------------------- | ---------------------------------------- |
| ✅ Frontend state sync     | `Subscribe()` per-entity or model   | ✅ Native sync, but JS-heavy              |
| ✅ Microservice queue      | `CatalogShiftExpired()` + callbacks | ❌ Not built for queues                   |
| ✅ Distributed lock logic  | Built-in cross-service locks        | ❌ Not possible                           |
| ✅ Embedded backend        | Fileless, RAM-only Swamp mode       | ❌ Requires Google Cloud and setup        |
| ✅ Domain event modeling   | Typed structs + TTL + emit hooks    | ⚠️ Needs custom logic and workarounds    |
| ✅ Mobile data persistence | Works with any gRPC-enabled backend | ✅ SDKs available, but offline is fragile |

---

## Why Firebase is fragile at scale

* ❌ **No exit path**: You build against their APIs and rules. Migration is painful.
* ❌ **No local-first dev**: You need the Firebase CLI and internet to even test things.
* ❌ **Expensive surprises**: "Free tier" is bait. Reads/writes scale poorly in price.
* ❌ **Weak for backend logic**: Firestore isn’t built for business logic or locks — just sync.
* ❌ **Hard to self-host**: There’s no easy local Firebase that mimics real behavior.
* ❌ **Proprietary CLI bloat**: Deployments tied to tooling, not your architecture.

---

## Terminology comparison

| HydrAIDE Term             | Firebase Equivalent       | Explanation                                    | HydrAIDE-Native Advantage                                   |
| ------------------------- | ------------------------- | ---------------------------------------------- | ----------------------------------------------------------- |
| **Swamp**                 | Collection / Path         | Logical container for data, typed, hash-routed | 🔹 No provisioning <br> 🔹 File-free or persistent          |
| **Treasure**              | Document / Record         | Typed struct value with key, TTL, events       | 🔹 Binary, locked, subscribable                             |
| **Key**                   | Path / Document ID        | Unique identifier in struct                    | 🔹 Used in locking, TTL, sharding                           |
| **Subscribe**             | onSnapshot / listeners    | Real-time event stream                         | 🔹 No client SDK needed <br> 🔹 Works from any gRPC service |
| **Profile**               | Flat user object          | Struct with per-field Treasures                | 🔹 Native save/read/update logic                            |
| **Lock()**                | Not available             | Domain-safe locking with auto-expiry           | 🔹 Works across services, TTL-based                         |
| **Catalog**               | Subcollection or index    | Streamable Swamp for filtered analytics        | 🔹 TTL, Beacon-ready, real-time read                        |
| **CatalogShiftExpired()** | Firebase Queue workaround | TTL-based queue pop + event callbacks          | 🔹 Safe & built-in queue logic                              |

---

## TL;DR

**Firebase is great for quick MVPs**, but it locks you into a managed, pricing-sensitive ecosystem where your app 
logic must dance around its limitations.

**HydrAIDE gives you back control**, even for frontend use cases, with real-time sync, 
structured typing, and serverless-grade latency, without ever touching a CLI or a bill from Google.

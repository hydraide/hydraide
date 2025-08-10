## HydrAIDE vs DynamoDB

**DynamoDB** is an **AWS-managed**, **schema-free** key-value store that scales well, but comes with trade-offs:

* It's locked into AWS,
* Costs can grow quickly with read/write throughput,
* And it lacks native type safety or reactivity.

**HydrAIDE**, by contrast, is a **self-hostable**, **logic-native**, and **per-key lockable** runtime for 
reactive systems — offering **native typing**, real-time subscriptions, and deterministic behavior, without vendor 
lock-in or unexpected bills.

---

## Feature comparison

| Feature            | HydrAIDE                               | DynamoDB                                   |
| ------------------ | -------------------------------------- | ------------------------------------------ |
| 📦 Storage model   | ✅ Typed binary structs         | ❌ JSON-like documents, no real type safety |
| 🔐 Locking         | ✅ `Lock()/Unlock()` per key            | ❌ No locking; app-level race prevention    |
| 🔄 Reactivity      | ✅ Built-in `Subscribe()` on change     | ❌ No pub/sub, only polling or Streams API  |
| 🧠 Type safety     | ✅ Compile-time enforced types       | ❌ Manual parsing from JSON                 |
| 🧹 TTL / Cleanup   | ✅ Native `expireAt` + `ShiftExpired()` | ⚠️ TTL via table config, no deletion hook  |
| 🚦 Sharding        | ✅ Deterministic folder-based hash      | ⚠️ Transparent, auto-sharded via AWS infra |
| 🧰 Deployment      | ✅ Self-hosted binary or Docker         | ❌ AWS-only, requires provisioning          |
| 💸 Cost scaling    | ✅ Flat infra, no write/RCU charges     | ❌ Pay-per-read/write + storage + streams   |
| 📦 Offline support | ✅ Works fully offline in self-hosted mode | ❌ Online-only; requires AWS endpoint |
| 🧗 Learning curve  | 🟢 Zero-to-Hero in 1 day                  | 🟡 AWS IAM, Streams, SDK boilerplate       |

---

## Use case comparison

| Use Case                  | HydrAIDE                               | DynamoDB                                              |
| ------------------------- | -------------------------------------- | ----------------------------------------------------- |
| ✅ Real-time subscriptions | `Subscribe()` from Go or gRPC          | ❌ Requires Streams + Lambda glue                      |
| ✅ Queues & timers         | `CatalogShiftExpired()` + callbacks    | ❌ Needs DynamoDB TTL + external Lambda                |
| ✅ Distributed locking     | `Lock()` per domain key                | ❌ Manual logic only, no native lock API               |
| ✅ Local-first development | Zero-setup binary mode                 | ❌ Requires AWS CLI + Docker emulator                  |
| ✅ Frontend event sync     | Works over gRPC, event-native          | ⚠️ Needs extra infra (API Gateway + Lambda + Streams) |
| ✅ Typed user profiles     | `ProfileRead/Save()` with struct logic | ⚠️ Needs custom struct parsing & schema               |

---

## Why DynamoDB is fragile at scale

* ❌ **Expensive scale-out**: write/read throughput is provisioned (RCU/WCU), and pricing explodes under traffic.
* ❌ **Limited reactivity**: needs DynamoDB Streams + Lambda + polling for any eventing.
* ❌ **Vendor lock-in**: AWS-only; migrating away is complex and slow.
* ❌ **Offline-dev unfriendly**: requires emulator setup; behaves differently from real infra.
* ❌ **No locking support**: application must implement race prevention manually.
* ❌ **Hidden complexity**: TTL, Streams, Lambdas, Global Tables = infrastructure maze.

---

## Terminology comparison

| HydrAIDE Term             | DynamoDB Equivalent      | Explanation                                | HydrAIDE-Native Advantage              |
| ------------------------- | ------------------------ | ------------------------------------------ | -------------------------------------- |
| **Swamp**                 | Table / Partition        | Logical container for structured Treasures | 🔹 Memory-based, reactive, file-free   |
| **Treasure**              | Item                     | Typed Go struct per key                    | 🔹 Binary, evented, TTL-aware          |
| **Key**                   | Partition/Sort key       | Unique identifier inside a Swamp           | 🔹 Used in locking, TTL, direct access |
| **Subscribe()**           | Streams + Lambda         | Reactive event handler                     | 🔹 Real-time, zero infra, no polling   |
| **Profile**               | Composite item per user  | Typed profile logic, field-per-field       | 🔹 Save/Read in one line, Go-native    |
| **Lock() / Unlock()**     | Not available            | Distributed locking mechanism              | 🔹 TTL-backed, cross-service safe      |
| **Catalog**               | Filtered item set (Scan) | Indexable, time-aware container            | 🔹 TTL, expiry queue, real-time read   |
| **CatalogShiftExpired()** | TTL scan + custom logic  | Evented pop queue with delay               | 🔹 Safe, lossless, atomic per item     |

---

## TL;DR

**DynamoDB** is a powerful NoSQL store, but it's tightly coupled to the AWS ecosystem, requires complex infra 
for reactivity or queues, and becomes cost-prohibitive under real-world load.

**HydrAIDE** gives you **type-safe**, **lockable**, **reactive** infrastructure without any cloud dependency, 
delivering serverless performance **on your terms**, without vendor traps or hidden complexity.

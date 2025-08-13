## 🌐 Effortless Scaling – Without an Orchestrator

When I designed HydrAIDE, one core goal was clear: **never require external orchestrators** to let the system know exactly where data resides across multiple servers.

Most traditional databases can’t do this because they store data in **non-deterministic ways** — often inside a large database file that must be managed by a separate coordination layer. That’s why they need external orchestration, metadata tables, replication, and sharding logic.

HydrAIDE is different:

* Every **Swamp** name is deterministically hashed to a specific **folder** (island).
* This hash works not only at folder level, but also **across multiple servers**.
* The Swamp name → hash → island ID → server chain tells you exactly which server holds the data, **without external metadata or a central coordinator**.

### 🧠 How does the “island” model work?

An **island** is a deterministic number (between 1–N) mapped directly to a folder on disk.
The client SDK hashes the full Swamp name, then calculates the island ID. These ID ranges are pre-assigned to servers:

* Server 1: islands 1–100
* Server 2: islands 101–200

This mapping **never changes** unless the total number of islands (AllIslands) changes. That’s why it’s best to start with a large value (e.g., 1000) to allow future scaling without data migration.

### ⚡ Why is it fast?

* **O(1)** time to calculate where a Swamp belongs.
* No intermediate layer, no query parser, no metadata lookup.
* The SDK connects directly to the correct server — up to N+1 servers, **without an orchestrator**.

### 🔗 Multi-server connection in the SDK

In `repo.New()`, you simply list each server’s address, TLS certificate, and island range. From that point, the client automatically knows:

1. On save → which server to send the request to.
2. On read → which server to query for the Swamp.

**Example:**

```go
repoInterface := repo.New([]*client.Server{
    {
        Host:       os.Getenv("HYDRA_HOST_1"),
        FromIsland: 1,
        ToIsland:   100,
        CertFilePath: os.Getenv("HYDRA_CERT_1"),
    },
    {
        Host:       os.Getenv("HYDRA_HOST_2"),
        FromIsland: 101,
        ToIsland:   200,
        CertFilePath: os.Getenv("HYDRA_CERT_2"),
    },
}, 200, 10485760, true)
```

This configuration sets up two servers that together handle 200 islands, with exact range boundaries.

### 🎯 Result

This philosophy and implementation:

* **Eliminates the need for central coordination**
* Enables easy horizontal scaling (new server = new island range)
* Maintains deterministic, fast access to any Swamp
* Delivers stable, predictable performance — even with massive data volumes and server counts

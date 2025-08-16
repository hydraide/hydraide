## Memory-Efficient Operation – Philosophy and Practical Benefits

One of the biggest limitations of traditional databases is that they **either keep everything in memory** or are fully **disk-based**. Developers rarely have the option to decide **what stays in memory and for how long**.

In real-world scenarios, this control is often crucial:

* A **user Swamp** – holding data needed for logins and listings – benefits from being in memory so every call doesn’t hit the disk.
* In contrast, a user’s **detailed profile** with hundreds of fields doesn’t need to stay in memory and can be loaded from SSD when needed.

### The Limitations of Traditional DBs

When processing millions of websites, I faced the same problem. To get fast access to terabytes of data in a traditional DB, you either load **everything into memory** (requiring many distributed servers) or rely on **slow, file-based storage**.

### The HydrAIDE Approach

HydrAIDE takes a different view: it **gives developers the freedom** to configure how long a Swamp remains in memory after last access.

This is controlled via the `CloseAfterIdle` setting, which – like everything else in HydrAIDE – can be set directly in code when registering a Swamp.

When a Swamp is first accessed (“hydrated”), it is loaded from SSD into memory. If unused for the configured period, it’s automatically removed from memory.

```go
CloseAfterIdle: time.Second * 21600, // Remove from memory after 6 hours idle
```

The Go garbage collector (GC) efficiently handles memory release — no extra background processes are required.

#### Practical Example – Registering a Swamp with Memory Management

```go
type CatalogModelUserSaveExample struct {
UserUUID  string    `hydraide:"key"`
Payload   *Payload  `hydraide:"value"`
CreatedBy string    `hydraide:"createdBy"`
CreatedAt time.Time `hydraide:"createdAt"`
UpdatedBy string    `hydraide:"updatedBy"`
UpdatedAt time.Time `hydraide:"updatedAt"`
}

type Payload struct {
LastLogin time.Time
IsBanned  bool
}

func (c *CatalogModelUserSaveExample) RegisterPattern(r repo.Repo) error {
ctx, cancel := hydraidehelper.CreateHydraContext()
defer cancel()

h := r.GetHydraidego()

errs := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
SwampPattern:    c.createCatalogName(), // e.g., users/catalog/all
CloseAfterIdle:  6 * time.Hour,         // Keep in RAM for 6 hours of inactivity
IsInMemorySwamp: false,                 // Persist to disk as well
FilesystemSettings: &hydraidego.SwampFilesystemSettings{
WriteInterval: 10 * time.Second,
MaxFileSize:   8 * 1024,
},
})

if errs != nil { return hydraidehelper.ConcatErrors(errs) }
return nil
}
```

This Catalog example shows how you **control memory lifetime (`CloseAfterIdle`)**, persistence, and write strategy in code. You can keep frequently used data in RAM for speed while loading less critical data from SSD on demand.

### Real-World Large Dataset Example

With this setting, I was able to handle **billions of word associations** for millions of websites from a single server, keeping search times under **1 second** even at massive scale. If you’re interested, you can read more about how I made Europe searchable from a single server here: [https://dev.to/hydraide/how-i-made-europe-searchable-from-a-single-server-the-story-of-hydraide-432h](https://dev.to/hydraide/how-i-made-europe-searchable-from-a-single-server-the-story-of-hydraide-432h)

### Benefits

* **Flexible memory use** – only keep what’s needed in RAM.
* **Cost efficiency** – a single server can handle terabytes of data in real time with O(1) access speed.
* **Optimized performance** – frequent data comes from memory; less-used data is hydrated from SSD.

---

This philosophy makes HydrAIDE fast, resource-efficient, and adaptable to developer needs.

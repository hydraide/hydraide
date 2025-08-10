## Database Engine – Philosophy and Operation

The HydrAIDE data management model is fundamentally different from classic database approaches. It uses no schemas, no query language (SELECT, WHERE, etc.), and no central query engine. Instead, the philosophy is:

**"The structure lives in the developer’s intent and type definition, not in configuration files."**

The primary reference SDK for HydrAIDE currently builds on Go’s strongly typed models: whatever the developer defines as a struct is stored by the system in binary form, with machine precision. This removes the migration burden of schema changes and makes the developer’s code itself the “query.” Alongside the Go SDK, a Python version is on the way, with official SDKs for other languages coming soon.

### Why was it designed this way?

HydrAIDE was originally created for a B2B search system that needed to store data from millions of websites and make it available in real time, without separate cache or pub/sub systems. Existing database technologies (SQL, NoSQL, Redis, Kafka) could not meet the combined requirements of speed, real-time reactivity, and memory efficiency. This led to a model where the **Swamp** — as both a physical and logical unit — defines the location, behavior, and lifecycle of data.

### How does it work in practice?

To store data, you simply define a Go struct, which itself becomes the Swamp — the storage unit — for example, a user profile:

```go
type UserProfile struct {
    Name   string
    Age    uint8
    Active bool
}
```

Then save it in Profile mode:

```go
profile := &UserProfile{
    Name:   "Péter",
    Age:    34,
    Active: true,
}

h.ProfileSave(ctx, name.New().Sanctuary("users").Realm("profiles").Swamp("peter"), profile)
```

No field definitions on the database side, no ALTER TABLE, no JSON conversion. No need to create tables and indexes in the database — everything stays in your code, and you work with your own Go structs. The stored data is saved in binary, type-safe form and loaded back exactly the same way.

**What happens on save?** HydrAIDE deterministically resolves the Swamp name, calculates the target folder/server, automatically creates the Swamp in microseconds if needed, stores the struct fields as individual binary **Treasures**, sends events to relevant Subscribers, and — if persistence is enabled — flushes the write to SSD in the background. A profile-level write like this is **extremely fast even on a single thread**: real-world measurements show **600–700k operations/sec** on a HydrAIDE server.

When you save the user, the Swamp is automatically created in microseconds and the data is instantly stored in it. The process is highly optimized: a single HydrAIDE server thread can handle 600–700k such writes per second.

### Advantages

* **Speed:** O(1) access time, no query parser.
* **Simplicity:** the code is the query.
* **Reactivity:** every change generates a real-time event.
* **Memory efficiency:** Swamps only live in memory when accessed.

This approach not only provides a database but also a **thinking framework** where the developer-defined structure is the actual live data model. To explore more of what HydrAIDE can do, check out the Go SDK documentation — this was only a very simple demonstration; the system offers much more.

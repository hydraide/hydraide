## Pure gRPC control — the protocol is the contract

### What this means

The HydrAIDE wire protocol is gRPC, defined by [`proto/hydraide.proto`](../../proto/hydraide.proto). Anything an SDK can do is a method on that proto — there are no SDK-only behaviours hiding behind extra abstractions.

Properties that follow from that:

* **Generate a client in any language.** A single `protoc` invocation produces a typed client in Go, Python, Node.js, Java, Rust, C#, or any other protoc-supported language.
* **End-to-end typed.** Field names and types come from the proto; no manual JSON parsing, no runtime casting.
* **HTTP/2 binary transport.** Multiplexed streams are used directly for the streaming reads (`GetByIndexStream`, `SubscribeToEvents`).
* **mTLS by default.** Cert-based authentication is part of the connection, not an afterthought.

HydrAIDE’s Go SDK is a **convenience wrapper** over these gRPC calls — adding features like:

* Direct struct handling (automatic (de)serialization of Go structs to binary Treasures)
* Name-based routing across multiple HydrAIDE servers via deterministic folder hash mapping
* Utility helpers for timeouts, Swamp name builders, and structured error handling

But here’s the key: **you never have to use the SDK**.
If you want to run ultra-lean — for example, on a Raspberry Pi edge node, an embedded controller, or a minimal CLI tool — you can skip the SDK entirely.
Just use the generated gRPC client for your language and call the service methods directly.

In practice this means:

* In a full-stack service, you can use the SDK for its struct/model handling.
* On a resource-constrained edge or IoT device, you can skip the SDK and call the protoc-generated client directly.
* In a multi-language stack, each team uses the language they're already productive in — there is no central translation layer to maintain.

---

### Why this matters in practice

* **No SDK lock-in.** A team that prefers Rust, Python, or Node can talk to a HydrAIDE server without waiting for a bespoke SDK.
* **Edge and embedded.** Skip the Go SDK on resource-constrained devices and use the raw protoc-generated client.
* **One contract for the whole stack.** Code review of new RPCs happens once — in the proto file — and propagates to every language.

📄 The `.proto` file with full documentation: [HydrAIDE protocol definition](../../proto/hydraide.proto)

## 🛰️ Pure gRPC Control — Fully SDK-Optional, Ultra-Fast, Language-Native

### Philosophy

From day one, HydrAIDE was built on a simple but non-negotiable principle:
**access to the engine must be possible from anywhere, in any language, with maximum speed and type safety — without extra layers.**

That’s why the foundation of HydrAIDE’s communication is **pure gRPC**, defined by `.proto` files.
The result is a transport that’s:

* **Build-once, run-anywhere** — generate native clients in Go, Python, Node.js, Java, Rust, C#, and more with a single `protoc` command.
* **Strongly typed end-to-end** — no guessing field names, no brittle JSON parsing, no runtime casting errors.
* **Ridiculously fast** — binary over HTTP/2 with multiplexed streams, optimized for low-latency edge and IoT scenarios.
* **Secure by default** — TLS-encrypted, cert-based access baked in.

HydrAIDE’s Go SDK is a **convenience wrapper** over these gRPC calls — adding features like:

* Direct struct handling (automatic (de)serialization of Go structs to binary Treasures)
* Name-based routing across multiple HydrAIDE servers via deterministic folder hash mapping
* Utility helpers for timeouts, Swamp name builders, and structured error handling

But here’s the key: **you never have to use the SDK**.
If you want to run ultra-lean — for example, on a Raspberry Pi edge node, an embedded controller, or a minimal CLI tool — you can skip the SDK entirely.
Just use the generated gRPC client for your language and call the service methods directly.

This makes HydrAIDE uniquely flexible:

* In **full-stack services**, you might use the SDK for its developer-friendly model handling.
* In **resource-constrained edge/IoT devices**, you might use raw gRPC for minimal overhead.
* In **multi-language ecosystems**, teams can connect in whatever language they’re productive in — with no translation layer.

---

### Why this matters

* **Zero lock-in** — Any language, any stack, any runtime.
* **Max performance** — No extra abstractions in the hot path.
* **Future-proof** — New languages get instant first-class support by regenerating protobuf clients.
* **Flexible architecture** — Choose between full-feature SDKs or bare-metal gRPC depending on the environment.

> HydrAIDE doesn’t just have an SDK — it *is* the protocol.
> Everything the SDK can do, you can do directly via gRPC.

---

📄 **The `.proto` file with full documentation is available here:** [HydrAIDE Protocol Definition](../../proto/hydraide.proto)

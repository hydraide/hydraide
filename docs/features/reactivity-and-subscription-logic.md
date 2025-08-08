## Built-in Reactivity – Philosophy and Operation

One of HydrAIDE’s core philosophical principles is that developers **want to write code, not manage infrastructure**. A modern developer shouldn’t have to become an infra manager or DevOps engineer just to build reactive, real-time systems. Instead, they need a tool that **has everything built-in** — especially reactivity — without having to stitch it together from separate layers.

### Why HydrAIDE is Different

In most reactive architectures, separate components handle event delivery and real-time updates — Redis Pub/Sub, Apache Kafka, RabbitMQ, or custom websocket/polling logic. These require extra infrastructure, configuration, and often background processes that **slow down the system** and consume resources.

HydrAIDE’s philosophy is that **everything happens inside the core database**, which is itself reactive — no external brokers, no separate pub/sub layer. This simplifies development and removes the need for parallel daemon processes that consume performance.

### How Reactivity Works

When any modification happens in a Swamp — **Create, Read, Update, or Delete** — HydrAIDE checks if there’s a subscriber for that Swamp.

* If no subscribers: no events are generated.
* If there are subscribers: HydrAIDE **immediately generates an event**, delivered to the subscriber’s client via a gRPC stream in near real time.

The `Subscribe` function is one of the most powerful HydrAIDE SDK primitives: it listens for Swamp-level changes and forwards them to a given iterator callback.

* If `getExistingData` is true, all existing data is sent first (ordered by creation time), then the live stream starts.
* For each event, a fresh instance of the given model is created, filled with the event data, and passed to the iterator.

### Example – Real-time Subscription (actual `Subscribe` signature)

```go
// type SubscribeIteratorFunc func(model any, eventStatus EventStatus, err error) error

type ChatMessage struct {
    ID      string
    User    string
    Message string
    SentAt  time.Time
}

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

swamp := name.New().Sanctuary("chat").Realm("messages").Swamp("room-42")

err := h.Subscribe(ctx, swamp, true, ChatMessage{}, func(model any, status EventStatus, iterErr error) error {
    if iterErr != nil {
        log.Printf("stream/convert error: %v", iterErr)
        return nil
    }
    msg := model.(*ChatMessage)
    switch status {
    case StatusNew:
        log.Printf("[NEW] %s: %s", msg.User, msg.Message)
    case StatusUpdated:
        log.Printf("[UPDATED] %s: %s", msg.User, msg.Message)
    case StatusDeleted:
        log.Printf("[DELETED] id=%s", msg.ID)
    case StatusNothingChanged:
        log.Printf("[SNAPSHOT] %s: %s", msg.User, msg.Message)
    default:
        log.Printf("[UNKNOWN] %v", status)
    }
    return nil
})
if err != nil {
    log.Fatalf("subscribe failed: %v", err)
}
```

With `getExistingData = true`, you first get all current records (as `StatusNothingChanged`), then live events. Each callback call provides a **freshly instantiated pointer** to the blueprint type and an `EventStatus`: `StatusNew`, `StatusUpdated`, `StatusDeleted`, or `StatusNothingChanged`.

### Behavior and Guarantees

* **Swamp-level stream**: every write/update/delete event is sent if there is at least one subscriber.
* **Snapshot + Live**: optional initial data load before streaming new changes.
* **Non-blocking**: runs in a background goroutine; stops when context is canceled, the iterator returns an error, or the server closes the stream.
* **Type-safe payloads**: iterator always receives a pointer to the specified non-pointer blueprint type.
* **Error handling**: conversion errors are passed to the iterator; can be non-fatal if handled.
* **Low latency**: gRPC streaming; <1 ms when Swamp is in memory.

### Benefits

* **Zero overhead**: no separate Redis/Kafka/MQ server.
* **Near-zero latency**: gRPC-speed event delivery.
* **Intelligent operation**: generates events only if someone is listening.
* **Simple code**: HydrAIDE SDK handles connection and event processing.

---

This approach gives developers a **true real-time database** for building dynamic dashboards, chat systems, microservices, or complex workflows — all inside a single integrated system, without unnecessary external components.

## Reactivity and subscriptions

Every write, update, and delete inside a Swamp emits an event. Clients subscribe to a Swamp and receive those events over a gRPC stream — there is no separate pub/sub broker to operate.

### How reactivity works

When a write, update, or delete happens in a Swamp, HydrAIDE checks whether the Swamp has any subscribers.

* If no subscribers: no events are generated.
* If there are subscribers: an event is emitted on the gRPC stream as the change is committed.

Reads do not emit events. Subscribe is a write-side notification mechanism, not an access log.

The `Subscribe` SDK call listens for Swamp-level changes and forwards them to an iterator callback.

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
* **In-memory delivery**: when the Swamp is hot, events flow over the gRPC stream without a disk round-trip on the read side.

### What this replaces

Subscriptions cover the same shape of work that Redis Pub/Sub, simple Kafka topics, or a custom WebSocket fan-out would handle: notify subscribers when something changed in a known namespace. They are not a durable work queue with retries, acknowledgements, and dead-letter handling — for that, run a queue (NATS JetStream, Kafka) alongside HydrAIDE.

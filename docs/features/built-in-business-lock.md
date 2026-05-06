# Business-level locks

## Philosophy

Imagine you’re working in a busy store, and several customers reach the cash register at the same time.
You wouldn’t want two people to access the till simultaneously — you need a rule that enforces order and ensures only one person handles the register at any moment.

The **HydrAIDE business-level lock** does exactly that, but in software form: whenever you need to run a critical business operation (for example, updating an account balance, refreshing stock levels, or processing a transaction), you can lock the given resource using a custom key (e.g., `user-123-account`).
This lock is completely independent from HydrAIDE’s internal per-Treasure lock and is designed specifically for safely queueing business processes.

## Operation

* The first client to call the **Lock()** method with a given key instantly acquires the lock and can execute its task.
* If a second client tries to lock the same key, the system **blocks the call** until the previous task finishes. Once the first client completes its work, the block on the second client is automatically lifted, and the system immediately hands over the lock — allowing the client’s process to continue without any extra listeners or special code.
* The lock works across processes and services that share the same HydrAIDE instance.

### TTL (Time-To-Live) Protection

Each lock has a **TTL timeout** configurable by the developer.
If a client crashes or fails to release the lock, the system automatically frees it once the TTL expires, ensuring that **no deadlock** can occur.

## Advantages

* Safe, order-controlled execution for critical business logic
* Deadlock-free operation with TTL
* Easy to use with minimal code requirements
* Can simulate transaction-like flows where multiple operations must run atomically

## How this differs from other approaches

If you have used distributed locks before, you have probably reached for one of these patterns:

* **Database row locks** (Postgres, MySQL): work well for transactional updates inside one DB connection, but they are tied to the lifetime of a transaction and do not generalise to "lock this business object across N services".
* **Redis `SETNX`** + token + Lua release: a common application-level lock, but you have to write the queueing and fairness logic yourself.
* **Memcached**: no built-in locking primitive — everything is on you.

The HydrAIDE business lock fills the same niche as the `SETNX`-pattern, with the queueing, TTL release, and FIFO ordering already implemented. If your application already talks to HydrAIDE, you don't run a separate lock service.

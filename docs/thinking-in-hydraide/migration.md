# 🧬 Migration in HydrAIDE – Struct Evolution Without Fear

HydrAIDE doesn’t require migrations like traditional databases do. There is no schema registry, no table diffing, no ALTER COLUMN statements. Instead, evolution is **code-driven**, **lazy**, and **binary-compatible**.

This section explains how and why data evolution in HydrAIDE is fundamentally simpler — and safer — than in schema-bound systems like PostgreSQL or MongoDB.

---

## ✅ Why You Don’t Need Migrations

* HydrAIDE stores **typed binary structs**, not dynamic schemas.
* If you **add a new field**, older data remains valid.
* If you **remove a field**, older data still loads into a trimmed struct.
* If you **rename a field**, it behaves like a new field.
* Structs are hydrated into memory only when accessed — and adapted lazily.

Every read operation is a chance to evolve.
No global lock. No data copy. No downtime.

---

## 🧠 Behavior by Design

HydrAIDE instances are stateless:

* There’s no central controller to coordinate migrations.
* Each instance accesses its own Swamps directly.
* Every struct is read into the latest Go type you define in your code.

As a result:

* 🚫 No centralized migration process is ever needed.
* 🤝 Mixed-version clients can safely co-exist.
* 🔄 Changes are applied **on read**, only when needed.

---

## 📦 Example: Evolving a UserProfile Struct

Let’s say your original struct looks like this:

```go
type UserProfile struct {
    UserID        string
    Email         string
    Username      string
    IsVerified    bool
    Age           uint8 `hydraide:"omitempty"`
    LastLoginAt   *time.Time `hydraide:"omitempty"`
}
```

You already have saved data. Now, you evolve it:

### ➕ Add Fields

```go
type UserProfile struct {
    UserID        string
    Email         string
    Username      string
    IsVerified    bool
    Age           uint8 `hydraide:"omitempty"`
    LastLoginAt   *time.Time `hydraide:"omitempty"`

    // New fields
    LoginCount    int32
    Rating        float64 `hydraide:"omitempty"`
    Avatar        *UserImage `hydraide:"omitempty"`
}
```

✅ HydrAIDE will load old data correctly. The new fields will be zero-valued.

### ➖ Remove Fields

If you remove `LastLoginAt`, old data will still deserialize — the field is simply ignored during read.

✅ You don’t need to re-save anything for the app to keep working — but if you want to remove legacy data from disk, you can do so manually via cleanup.

### 🔁 Rename Fields

Renaming `Age` to `YearsOld` creates a new field.

⚠️ This does not carry over old values automatically. Treat renames as additive.

---

## 🔄 Optional: Manual Cleanup or Migration

While HydrAIDE makes migrations unnecessary, you **can** explicitly rewrite old data if desired.

> Note: Cleanup and migration can be performed incrementally. There’s no need to process all Swamps at once — you can clean as they’re accessed in production.

### 🔃 Field-by-Field Cleanup

If you want to remove a deprecated field and eliminate its data:

1. Iterate all relevant Swamps.
2. Load each `UserProfile`.
3. Transfer the value to the new field (if renamed).
4. Set the old field to zero/nil.
5. Save the struct back.

#### 🧪 Example:

```go
if profile.Age != 0 {
    profile.YearsOld = int(profile.Age)
    profile.Age = 0
    _ = profile.Save(repo) // replace with your save logic
}
```

This ensures that legacy data is overwritten or cleared intentionally.

### 🔁 Full Swamp Migration

If your struct changes dramatically:

* Create a **new Swamp** (e.g. `user/profiles-v2/...`).
* Read entries from the old Swamp.
* Transform them into the new struct.
* Save into the new Swamp.
* Once verified, **Destroy()** the old Swamp.

This approach provides full data isolation and a clean structural restart.

---

### ⚠️ Changing Field Types

Changing the type of an existing field (e.g. `int32` → `string`) breaks backward compatibility.

If needed, introduce a new field with a new name, copy the value, and deprecate the old one explicitly.

---

## 🧰 How HydrAIDE Handles This Internally

* Data is saved in **binary chunks** with field tags.
* Each field is encoded with a tag derived from the struct field name.
* On load, HydrAIDE matches fields by tag.
* Unknown fields are skipped.
* Missing fields are left at zero or nil.

This allows HydrAIDE to support **partial deserialization**, **forward compatibility**, and **lazy evolution**.

---

## 🧪 Real-World Save / Load Pattern

Here’s how a struct typically interacts with the SDK:

```go
func (m *UserProfile) Save(r repo.Repo) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()
    return r.GetHydraidego().ProfileSave(ctx, m.createName(m.UserID), m)
}

func (m *UserProfile) Load(r repo.Repo) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()
    return r.GetHydraidego().ProfileRead(ctx, m.createName(m.UserID), m)
}
```

No versioning.
No migrations.
Just structs that evolve with your code.

---

## 🧨 What Not to Do

* ❌ Do not rely on struct tags like `json:"..."` — only HydrAIDE’s own tags matter.
* ❌ Do not attempt manual file migrations. Let the system handle reads naturally.
* ❌ Do not break backwards compatibility intentionally (e.g. changing types of existing fields).

---

## 🎯 Best Practices

* Use `hydraide:"omitempty"` on all optional fields.
* Group nested optional logic in pointer structs.
* Consider splitting structs if they grow large and are only partially used.
* Use the `RegisterSwamp()` pattern to explicitly configure persistence + TTL.

---

## 🤯 Summary

HydrAIDE is one of the few systems where:

* You can ship a new version without rewriting old data.
* You can read old Swamps with new code, instantly.
* You don’t need to version schemas, run migrations, or stop your system.

But if you **do** want to migrate or clean up data:

* You can iterate Swamp and Treasures, transform structs, and re-save.
* You can copy between Swamps, validate results, and destroy legacy storage.

> Traditional DBs need schema migrations. HydrAIDE needs only **intent**.

This is migration redefined:

> Lazy. Safe. Invisible. Intentional — and reversible if needed.

Welcome to the **evolution-first mindset**.

## 🧭 Navigation

← [🧹 Clean System](/docs/thinking-in-hydraide/clean-system.md) | [🌐 Distributed Architecture](/docs/thinking-in-hydraide/distributed-architecture.md)

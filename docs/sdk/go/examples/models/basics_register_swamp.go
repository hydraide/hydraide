package models

import (
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// BasicsRegisterSwamp demonstrates how to register a Swamp pattern
// with custom memory and filesystem behavior.
//
// 🧠 The registration should be called once at application startup,
// before any operations are performed on the target Swamp.
type BasicsRegisterSwamp struct {
	MyModelKey   string `hydraide:"key"`   // This field will be used as the Treasure key
	MyModelValue string `hydraide:"value"` // This field can hold any value, not used in this method
}

func (m *BasicsRegisterSwamp) RegisterPattern(repo repo.Repo) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := repo.GetHydraidego()

	// Registering a Swamp pattern allows you to configure its behavior
	// individually or by wildcard pattern. Unlike central configuration files,
	// HydrAIDE lets you control each Swamp directly via code.
	//
	// This registration method should be called:
	// - Once at application startup
	// - Before writing or reading any data into the target Swamp
	//
	// ✅ Safe to call multiple times — HydrAIDE will only apply changes if the config is different.
	//
	// 🔄 If you re-register with different settings, changes take effect immediately.
	//
	// ⚠️ Some settings behave differently:
	// - Chunk size only applies to *new* chunks. Existing ones are unaffected.
	// - Switching from disk-based to in-memory does NOT delete existing SSD data.
	//   → If you want a clean switch, call `Destroy()` before registering as in-memory.

	// RegisterSwamp always returns a []error.
	// Each error (if any) represents a failure during Swamp registration on a HydrAIDE server.
	//
	// ⚠️ Even when only a single Swamp pattern is registered, HydrAIDE may attempt to replicate or validate
	// the pattern across multiple server nodes (depending on your cluster).
	//
	// ➕ Return behavior:
	// - If all servers succeeded → returns nil
	// - If one or more servers failed → returns a non-nil []error
	//
	// 🧠 To convert this into a single `error`, you can use the helper:
	//     hydraidehelper.ConcatErrors(errorResponses)
	errorResponses := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{

		// SwampPattern defines which Swamps this config will apply to.
		//
		// You can define either a wildcard-based pattern or a fully qualified pattern.
		//
		// ✅ Wildcard-based pattern:
		//   Useful for profile Swamps or any group of Swamps that should share the same behavior.
		//   Example:
		//     name.New().Sanctuary("users").Realm("profiles").Swamp("*")
		//
		//   This pattern will apply to:
		//     users/profiles/petergebri
		//     users/profiles/fruzsigebri
		//     users/profiles/anyOtherUser
		//
		// ✅ Absolute pattern:
		//   Use this when registering one specific Swamp with a distinct behavior.
		//   Ideal for catalog Swamps, analytics buckets, etc.
		//   Examples:
		//     name.New().Sanctuary("users").Realm("registered").Swamp("all")
		//     name.New().Sanctuary("users").Realm("registrations-by-months").Swamp("2025-05")
		//
		// 🧪 Real-world example (Profile Swamps):
		// You want to apply the same settings to all user profiles:
		//   SwampPattern: name.New().Sanctuary("users").Realm("profiles").Swamp("*")
		//
		// 🧪 Real-world example (Catalog Swamps):
		// You want to treat registration buckets separately:
		//   SwampPattern: name.New().Sanctuary("users").Realm("registrations-by-months").Swamp("2025-05")
		SwampPattern: name.New().Sanctuary("MySanctuary").Realm("MyRealm").Swamp("*"),

		// CloseAfterIdle defines how long the Swamp stays in memory after the last access.
		//
		// When a Swamp is accessed, it is loaded ("hydrated") into memory.
		// This timer starts after the last access. Once it expires, the Swamp is closed and flushed to disk.
		//
		// ✅ High values keep the Swamp resident for high-performance workloads.
		// ✅ Low values ensure memory is quickly reclaimed.
		//
		// 🧪 Example (frequently accessed):
		// A Swamp that holds active user sessions, called every few seconds:
		//   CloseAfterIdle: 6 hours → avoids rehydration
		//
		// 🧪 Example (rarely used profiles):
		// A Swamp that only holds static user profile data:
		//   CloseAfterIdle: 1 second → efficient, memory-safe
		//
		// ❗ When a Swamp closes, it writes out any unsaved data to disk.
		CloseAfterIdle: time.Second * time.Duration(21600), // 6 hours

		// IsInMemorySwamp defines whether this Swamp should persist to disk or exist only in memory.
		//
		// ✅ If true:
		//   - The Swamp lives only in RAM.
		//   - When closed, all data is lost.
		//   - Great for caches, brokers, or volatile queues.
		//
		// ✅ If false:
		//   - Data is persisted to disk.
		//   - FilesystemSettings must be defined.
		//
		// 🧪 Example (ephemeral Swamp):
		// You’re building a temporary message buffer:
		//   IsInMemorySwamp: true
		//
		// 🧪 Example (durable Swamp):
		// You’re tracking registered users:
		//   IsInMemorySwamp: false
		IsInMemorySwamp: false,

		// FilesystemSettings must be provided when the Swamp is persistent.
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{

			// WriteInterval defines how often data is flushed from memory to disk.
			//
			// ✅ Lower = safer (but slower)
			// ✅ Higher = faster (but riskier)
			//
			// 🧪 Example:
			//   WriteInterval: 1s → frequent flush (default)
			//
			// ❗ Special case:
			//   0 means write immediately on every change — discouraged under heavy load.
			WriteInterval: time.Second * 1,

			// 💾 MaxFileSize — Controls how large each chunk file can grow on disk.
			//
			// 🔒 IMPORTANT:
			// → MaxFileSize should **NEVER** be smaller than your OS filesystem's minimum block size.
			//
			// 📈 If your Swamp contains *rarely changing data* (e.g. user profiles):
			// → Use a **larger** chunk size 1MB or above.
			// ✅ Benefits:
			//    - Fewer chunk files
			//    - Faster read performance
			//
			// 🔁 If your Swamp data is *frequently updated, modified, or deleted*:
			// → Use the **smallest safe** chunk size possible. 8 KB is a good default.
			// ✅ Benefits:
			//    - Only small disk blocks are modified
			//    - Minimizes SSD wear
			//    - Reduces write amplification
			//
			// 🧠 Filesystem block size reference:
			// - 🪟 Windows NTFS: 64 KB (partitions >128 GB)
			// - 🐧 Linux ext4/xfs: 4 KB
			// - 🍎 macOS APFS: 4 KB
			// - 🧱 ZFS: configurable (8–128 KB)
			//
			// ⚙️ SSD write performance guidance:
			// - Most modern SSDs are optimal with **8–64 KB block writes**
			//
			// ✅ Recommended default: **8 KB**
			//    - 🧬 Small enough to avoid write amplification
			//    - 🚀 Large enough for efficient sequential reads
			//    - 🔗 Well-aligned with nearly all operating systems
			//
			// ⬆️ You may increase to **64 KB** for large sequential Swamps (e.g. on NTFS)
			//
			// ❌ Never go below your OS filesystem's block size
			MaxFileSize: 8192, // 8 KB
		},
	})

	// If multiple HydrAIDE servers are involved and wildcards are used,
	// RegisterSwamp may return multiple error responses.
	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}

	return nil
}

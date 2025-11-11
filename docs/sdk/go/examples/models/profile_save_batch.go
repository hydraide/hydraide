package models

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
)

// ProfileSaveBatchExample demonstrates how to efficiently save multiple Profile Swamps in a single operation.
//
// 🎯 Problem:
// When you have 50, 100, or even 1000 user profiles to save, calling ProfileSave() in a loop
// results in multiple network round-trips, which is slow and inefficient:
//
//	for _, profile := range profiles {
//	    err := client.ProfileSave(ctx, name.New().Sanctuary("users").Realm("profiles").Swamp(profile.UserID), profile)
//	    // ... 100 network calls = very slow!
//	}
//
// 🚀 Solution: ProfileSaveBatch
// Instead of calling ProfileSave() repeatedly, you can batch all the profiles together
// and save them in **one gRPC call** (or a few calls if they span multiple servers),
// dramatically reducing latency and improving performance.
//
// 📊 Performance Comparison:
// ┌─────────────────────────────────────────────────────────────────┐
// │  Method           │ Swamps  │ Network Calls │ Estimated Time   │
// ├───────────────────┼─────────┼───────────────┼──────────────────┤
// │  ProfileSave loop │   100   │     100+      │  ~1000-2000 ms   │
// │  ProfileSaveBatch │   100   │      1-3      │   ~20-50 ms      │
// └─────────────────────────────────────────────────────────────────┘
//
// 💡 20-50x faster for bulk profile saving!
//
// ----------------------------------------------------
// 📦 When to use ProfileSaveBatch:
// ✅ Bulk user profile creation or updates
// ✅ Data import or migration tasks
// ✅ Batch updates after processing (e.g., nightly score updates)
// ✅ Saving settings for multiple entities at once
// ✅ Any scenario where you need to save many Profile Swamps efficiently
//
// ⚠️ When NOT to use ProfileSaveBatch:
// ❌ Saving a single profile (use ProfileSave instead)
// ❌ When you need transactional guarantees across profiles (HydrAIDE doesn't support multi-Swamp transactions)
//
// ----------------------------------------------------
// 🔧 How it works:
// 1. Create parallel lists of Swamp names and models (must be same length)
// 2. Call ProfileSaveBatch with both lists and an iterator function
// 3. The function groups operations by target server for optimal routing
// 4. Handles deletable fields automatically (deletes empty deletable fields)
// 5. The iterator is called once per profile with success or error status
//
// ----------------------------------------------------
// 📝 Example Usage:
func ProfileSaveBatchExample(client hydraidego.Hydraidego) {
	ctx := context.Background()

	// Example 1: Define a user profile model
	type UserProfile struct {
		Username      string    `hydraide:"Username"`
		Email         string    `hydraide:"Email"`
		FullName      string    `hydraide:"FullName,omitempty"`
		Age           int       `hydraide:"Age,omitempty"`
		IsActive      bool      `hydraide:"IsActive"`
		LastLoginTime time.Time `hydraide:"LastLoginTime"`
		Score         float64   `hydraide:"Score,omitempty,deletable"` // Can be deleted if set to 0
	}

	// Prepare the profiles to save
	profiles := []UserProfile{
		{Username: "alice", Email: "alice@example.com", FullName: "Alice Smith", Age: 25, IsActive: true, LastLoginTime: time.Now(), Score: 100.5},
		{Username: "bob", Email: "bob@example.com", FullName: "Bob Jones", Age: 30, IsActive: true, LastLoginTime: time.Now(), Score: 250.75},
		{Username: "charlie", Email: "charlie@example.com", FullName: "Charlie Brown", Age: 35, IsActive: false, LastLoginTime: time.Now(), Score: 150.0},
		// ... imagine 47 more users here
	}

	// Build parallel lists of swamp names and models
	swampNames := make([]name.Name, 0, len(profiles))
	models := make([]any, 0, len(profiles))

	for _, profile := range profiles {
		swampNames = append(swampNames, name.New().Sanctuary("users").Realm("profiles").Swamp(profile.Username))
		// Must pass pointer to struct
		profileCopy := profile
		models = append(models, &profileCopy)
	}

	// Save all profiles in batch
	var successCount int
	var errorCount int

	err := client.ProfileSaveBatch(ctx, swampNames, models, func(swampName name.Name, err error) error {
		if err != nil {
			errorCount++
			log.Printf("❌ Failed to save profile for %s: %v", swampName.Get(), err)
			return nil // Continue processing other profiles
		}
		successCount++
		log.Printf("✅ Saved profile for %s", swampName.Get())
		return nil
	})

	if err != nil {
		log.Fatalf("ProfileSaveBatch failed: %v", err)
	}

	// Print summary
	fmt.Printf("\n📊 Batch Save Summary:\n")
	fmt.Printf("  Total requested: %d\n", len(swampNames))
	fmt.Printf("  Successfully saved: %d\n", successCount)
	fmt.Printf("  Errors: %d\n", errorCount)
}

// ProfileSaveBatchUpdateExample demonstrates updating existing profiles in batch
func ProfileSaveBatchUpdateExample(client hydraidego.Hydraidego) {
	ctx := context.Background()

	type UserProfile struct {
		Username      string    `hydraide:"Username"`
		Email         string    `hydraide:"Email"`
		LastLoginTime time.Time `hydraide:"LastLoginTime"`
		LoginCount    int       `hydraide:"LoginCount"`
	}

	// Simulate updating login timestamps for multiple users after a batch login event
	usernames := []string{"alice", "bob", "charlie", "diana", "eve"}

	swampNames := make([]name.Name, 0, len(usernames))
	models := make([]any, 0, len(usernames))

	now := time.Now().UTC()
	for i, username := range usernames {
		swampNames = append(swampNames, name.New().Sanctuary("users").Realm("profiles").Swamp(username))
		models = append(models, &UserProfile{
			Username:      username,
			Email:         fmt.Sprintf("%s@example.com", username),
			LastLoginTime: now,
			LoginCount:    100 + i*10, // Simulated login counts
		})
	}

	var successCount int

	err := client.ProfileSaveBatch(ctx, swampNames, models, func(swampName name.Name, err error) error {
		if err != nil {
			log.Printf("❌ Failed to update %s: %v", swampName.Get(), err)
			return nil
		}
		successCount++
		return nil
	})

	if err != nil {
		log.Fatalf("Batch update failed: %v", err)
	}

	fmt.Printf("✅ Successfully updated %d user profiles\n", successCount)
}

// ProfileSaveBatchDeletableExample demonstrates the deletable field behavior
func ProfileSaveBatchDeletableExample(client hydraidego.Hydraidego) {
	ctx := context.Background()

	type GameProfile struct {
		PlayerID        string  `hydraide:"PlayerID"`
		CurrentLevel    int     `hydraide:"CurrentLevel"`
		Experience      int     `hydraide:"Experience"`
		BonusMultiplier float64 `hydraide:"BonusMultiplier,omitempty,deletable"` // Temporary bonus
	}

	// First, create profiles with bonuses
	playerIDs := []string{"player001", "player002", "player003"}

	// Initial save with bonuses
	swampNames := make([]name.Name, 0, len(playerIDs))
	models := make([]any, 0, len(playerIDs))

	for _, playerID := range playerIDs {
		swampNames = append(swampNames, name.New().Sanctuary("games").Realm("profiles").Swamp(playerID))
		models = append(models, &GameProfile{
			PlayerID:        playerID,
			CurrentLevel:    10,
			Experience:      5000,
			BonusMultiplier: 2.5, // Active bonus
		})
	}

	err := client.ProfileSaveBatch(ctx, swampNames, models, func(swampName name.Name, err error) error {
		if err != nil {
			log.Printf("Failed to save %s: %v", swampName.Get(), err)
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Initial save failed: %v", err)
	}

	fmt.Println("✅ Saved profiles with bonus multipliers")

	// Now remove bonuses by setting to 0 (deletable will trigger deletion)
	modelsUpdate := make([]any, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		modelsUpdate = append(modelsUpdate, &GameProfile{
			PlayerID:        playerID,
			CurrentLevel:    11, // Level up
			Experience:      6000,
			BonusMultiplier: 0, // Will be DELETED because of deletable tag
		})
	}

	err = client.ProfileSaveBatch(ctx, swampNames, modelsUpdate, func(swampName name.Name, err error) error {
		if err != nil {
			log.Printf("Failed to update %s: %v", swampName.Get(), err)
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Update save failed: %v", err)
	}

	fmt.Println("✅ Updated profiles - bonus multipliers removed via deletable tag")
}

// ProfileSaveBatchWithAbortExample shows how to abort on critical error
func ProfileSaveBatchWithAbortExample(client hydraidego.Hydraidego) {
	ctx := context.Background()

	type CriticalConfig struct {
		ConfigID   string `hydraide:"ConfigID"`
		APIKey     string `hydraide:"APIKey"`
		SecretHash string `hydraide:"SecretHash"`
	}

	configIDs := []string{"config-prod-1", "config-prod-2", "config-prod-3"}

	swampNames := make([]name.Name, 0, len(configIDs))
	models := make([]any, 0, len(configIDs))

	for _, configID := range configIDs {
		swampNames = append(swampNames, name.New().Sanctuary("system").Realm("configs").Swamp(configID))
		models = append(models, &CriticalConfig{
			ConfigID:   configID,
			APIKey:     "secret-key-" + configID,
			SecretHash: "hash-" + configID,
		})
	}

	var savedCount int

	err := client.ProfileSaveBatch(ctx, swampNames, models, func(swampName name.Name, err error) error {
		if err != nil {
			// For critical configs, abort on first error
			return fmt.Errorf("critical error saving config %s: %w", swampName.Get(), err)
		}
		savedCount++
		return nil
	})

	if err != nil {
		log.Fatalf("Failed to save critical configs: %v", err)
	}

	fmt.Printf("✅ Successfully saved %d critical configurations\n", savedCount)
}

// ProfileSaveBatchPartialUpdateExample demonstrates selective field updates
func ProfileSaveBatchPartialUpdateExample(client hydraidego.Hydraidego) {
	ctx := context.Background()

	type UserSettings struct {
		Theme         string `hydraide:"Theme"`
		Language      string `hydraide:"Language"`
		Notifications bool   `hydraide:"Notifications"`
		DarkMode      bool   `hydraide:"DarkMode"`
	}

	// Batch update: enable dark mode for all users
	userIDs := []string{"user001", "user002", "user003", "user004", "user005"}

	swampNames := make([]name.Name, 0, len(userIDs))
	models := make([]any, 0, len(userIDs))

	for _, userID := range userIDs {
		swampNames = append(swampNames, name.New().Sanctuary("user-settings").Realm("preferences").Swamp(userID))
		// Note: Only setting the fields we want to update
		// Other fields will be overwritten with zero values (this is ProfileSave behavior)
		// If you need partial updates, consider using a different strategy
		models = append(models, &UserSettings{
			Theme:         "default",
			Language:      "en",
			Notifications: true,
			DarkMode:      true, // The field we're updating
		})
	}

	var updatedCount int

	err := client.ProfileSaveBatch(ctx, swampNames, models, func(swampName name.Name, err error) error {
		if err != nil {
			log.Printf("Failed to update settings for %s: %v", swampName.Get(), err)
			return nil
		}
		updatedCount++
		return nil
	})

	if err != nil {
		log.Fatalf("Batch settings update failed: %v", err)
	}

	fmt.Printf("✅ Updated dark mode setting for %d users\n", updatedCount)
}

// ----------------------------------------------------
// 📚 Key Takeaways:
//
// 1️⃣ ProfileSaveBatch saves multiple Profile Swamps in ONE or FEW network calls (grouped by server)
// 2️⃣ swampNames and models must have the same length and correspond by index
// 3️⃣ Use the iterator function to track success/failure per profile
// 4️⃣ Return nil from iterator to continue, return error to abort
// 5️⃣ Deletable fields are automatically cleaned up when set to zero/empty
// 6️⃣ Perfect for bulk imports, migrations, batch updates, and synchronized saves
// 7️⃣ Can improve performance by 20-50x compared to loop-based ProfileSave
//
// 🎯 Best Practices:
// ✅ Always check for errors in the iterator
// ✅ Use meaningful Swamp naming conventions
// ✅ Pass pointers to structs in the models slice
// ✅ Consider the deletable tag for temporary fields that should be removed when empty
// ✅ Group related updates together for maximum efficiency
// ✅ Log save failures for monitoring and debugging
//
// ⚠️ Important Notes:
// - Each model must be a pointer to a struct
// - The iterator is called in the same order as swampNames
// - If a model conversion fails, the iterator receives an error for that specific profile
// - Empty swampNames or models list will return an error immediately
// - Iterator function must not be nil
// - Length mismatch between swampNames and models will return an error
//
// 🚀 Performance Tips:
// - Batch size of 50-200 Swamps works well in practice
// - For very large batches (1000+), consider splitting into chunks
// - Operations are automatically grouped by target server for optimal routing
// - Deletable field cleanup happens first, then all Set operations
// - Consider using context with timeout for large batch operations
//
// 💡 Comparison with ProfileReadBatch:
// - ProfileReadBatch: Load multiple profiles (read-only)
// - ProfileSaveBatch: Save multiple profiles (write operation)
// - Both use similar patterns and offer similar performance benefits
// - Both support batch operations across multiple servers transparently

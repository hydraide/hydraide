package models

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
)

// ProfileReadBatchExample demonstrates how to efficiently load multiple Profile Swamps in a single operation.
//
// 🎯 Problem:
// When you have 50, 100, or even 1000 user profiles to load, calling ProfileRead() in a loop
// results in multiple network round-trips, which is slow and inefficient:
//
//	for _, userID := range userIDs {
//	    user := &UserProfile{}
//	    err := client.ProfileRead(ctx, name.New().Sanctuary("users").Realm("profiles").Swamp(userID), user)
//	    // ... 100 network calls = very slow!
//	}
//
// 🚀 Solution: ProfileReadBatch
// Instead of calling ProfileRead() repeatedly, you can batch all the Swamp names together
// and load them in **one gRPC call**, dramatically reducing latency and improving performance.
//
// 📊 Performance Comparison:
// ┌─────────────────────────────────────────────────────────────────┐
// │  Method           │ Swamps  │ Network Calls │ Estimated Time   │
// ├───────────────────┼─────────┼───────────────┼──────────────────┤
// │  ProfileRead loop │   100   │     100       │  ~500-1000 ms    │
// │  ProfileReadBatch │   100   │      1        │   ~10-20 ms      │
// └─────────────────────────────────────────────────────────────────┘
//
// 💡 50-100x faster for bulk profile loading!
//
// ----------------------------------------------------
// 📦 When to use ProfileReadBatch:
// ✅ Loading user profiles for a dashboard or analytics page
// ✅ Bulk export or data migration tasks
// ✅ Fetching settings for multiple entities at once
// ✅ Any scenario where you need to load many Profile Swamps efficiently
//
// ⚠️ When NOT to use ProfileReadBatch:
// ❌ Loading a single profile (use ProfileRead instead)
// ❌ When you need to process profiles one-by-one with custom logic per profile
//
//	(though you can still use the iterator for this)
//
// ----------------------------------------------------
// 🔧 How it works:
// 1. Create a list of Swamp names (e.g., user IDs)
// 2. Call ProfileReadBatch with the list, a model template, and an iterator function
// 3. The iterator is called once per profile with the populated model
// 4. Handle errors gracefully (e.g., if a Swamp doesn't exist)
//
// ----------------------------------------------------
// 📝 Example Usage:
func ProfileReadBatchExample(client hydraidego.Hydraidego) {
	ctx := context.Background()

	// Example 1: Define a simple user settings profile model
	type UserSettings struct {
		Theme         string `hydraide:"Theme"`
		Language      string `hydraide:"Language"`
		Notifications bool   `hydraide:"Notifications"`
		FontSize      int    `hydraide:"FontSize"`
	}

	// Simulate having 50 user IDs to load
	userIDs := []string{
		"user-alice", "user-bob", "user-charlie", "user-diana", "user-eve",
		// ... imagine 45 more users here
	}

	// Build the list of Swamp names
	swampNames := make([]name.Name, 0, len(userIDs))
	for _, userID := range userIDs {
		swampNames = append(swampNames, name.New().Sanctuary("user-settings").Realm("profiles").Swamp(userID))
	}

	// Storage for loaded profiles
	var loadedProfiles []*UserSettings
	var errorCount int
	var successCount int

	// Call ProfileReadBatch with an iterator function
	err := client.ProfileReadBatch(ctx, swampNames, &UserSettings{}, func(swampName name.Name, model any, err error) error {
		if err != nil {
			// Handle error (e.g., Swamp doesn't exist)
			errorCount++
			log.Printf("❌ Failed to load profile for %s: %v", swampName.Get(), err)
			return nil // Continue processing other profiles
		}

		// Successfully loaded - type assert the model
		settings := model.(*UserSettings)
		loadedProfiles = append(loadedProfiles, settings)
		successCount++

		// Optional: Log or process each profile
		log.Printf("✅ Loaded profile for %s: Theme=%s, Language=%s", swampName.Get(), settings.Theme, settings.Language)

		return nil // Continue to next profile
	})

	if err != nil {
		log.Fatalf("ProfileReadBatch failed: %v", err)
	}

	// Print summary
	fmt.Printf("\n📊 Batch Load Summary:\n")
	fmt.Printf("  Total requested: %d\n", len(swampNames))
	fmt.Printf("  Successfully loaded: %d\n", successCount)
	fmt.Printf("  Errors: %d\n", errorCount)
}

// ProfileReadBatchAdvancedExample demonstrates advanced usage with error handling and filtering
func ProfileReadBatchAdvancedExample(client hydraidego.Hydraidego) {
	ctx := context.Background()

	// Define a more complex user profile
	type UserProfile struct {
		Username      string    `hydraide:"Username"`
		Email         string    `hydraide:"Email"`
		FullName      string    `hydraide:"FullName"`
		IsActive      bool      `hydraide:"IsActive"`
		LastLoginTime time.Time `hydraide:"LastLoginTime"`
		AccountLevel  int       `hydraide:"AccountLevel"`
	}

	// Load profiles for a specific set of users
	userIDs := []string{"alice", "bob", "charlie", "diana", "eve", "frank"}

	swampNames := make([]name.Name, 0, len(userIDs))
	for _, userID := range userIDs {
		swampNames = append(swampNames, name.New().Sanctuary("users").Realm("profiles").Swamp(userID))
	}

	// Use a map to store results by username
	profileMap := make(map[string]*UserProfile)
	var missingProfiles []string

	err := client.ProfileReadBatch(ctx, swampNames, &UserProfile{}, func(swampName name.Name, model any, err error) error {
		if err != nil {
			// Track which profiles are missing
			missingProfiles = append(missingProfiles, swampName.Get())
			return nil // Continue with other profiles
		}

		profile := model.(*UserProfile)

		// Optional: Filter by criteria (e.g., only load active users)
		if !profile.IsActive {
			log.Printf("⚠️  Skipping inactive user: %s", profile.Username)
			return nil
		}

		// Store in map for easy lookup
		profileMap[profile.Username] = profile

		return nil
	})

	if err != nil {
		log.Fatalf("ProfileReadBatch failed: %v", err)
	}

	// Process results
	fmt.Printf("\n✅ Loaded %d active profiles\n", len(profileMap))
	fmt.Printf("❌ Missing profiles: %v\n", missingProfiles)

	// Access specific profiles
	if alice, ok := profileMap["alice"]; ok {
		fmt.Printf("\nAlice's profile:\n")
		fmt.Printf("  Email: %s\n", alice.Email)
		fmt.Printf("  Last login: %s\n", alice.LastLoginTime.Format(time.RFC3339))
		fmt.Printf("  Account level: %d\n", alice.AccountLevel)
	}
}

// ProfileReadBatchWithAbortExample shows how to abort processing on critical error
func ProfileReadBatchWithAbortExample(client hydraidego.Hydraidego) {
	ctx := context.Background()

	type AdminSettings struct {
		Role        string `hydraide:"Role"`
		Permissions string `hydraide:"Permissions"`
	}

	adminIDs := []string{"admin-1", "admin-2", "admin-3"}

	swampNames := make([]name.Name, 0, len(adminIDs))
	for _, adminID := range adminIDs {
		swampNames = append(swampNames, name.New().Sanctuary("admin").Realm("settings").Swamp(adminID))
	}

	var admins []*AdminSettings

	err := client.ProfileReadBatch(ctx, swampNames, &AdminSettings{}, func(swampName name.Name, model any, err error) error {
		if err != nil {
			// For critical resources like admin settings, abort on first error
			return fmt.Errorf("critical error loading admin settings for %s: %w", swampName.Get(), err)
		}

		settings := model.(*AdminSettings)
		admins = append(admins, settings)

		return nil
	})

	if err != nil {
		log.Fatalf("Failed to load admin settings: %v", err)
	}

	fmt.Printf("✅ Successfully loaded %d admin profiles\n", len(admins))
}

// ----------------------------------------------------
// 📚 Key Takeaways:
//
// 1️⃣ ProfileReadBatch loads multiple Profile Swamps in ONE network call
// 2️⃣ Use the iterator function to process each loaded profile
// 3️⃣ Return nil from iterator to continue, return error to abort
// 4️⃣ Handle missing Swamps gracefully via error parameter
// 5️⃣ Perfect for bulk loading, dashboards, analytics, and data export
// 6️⃣ Can improve performance by 50-100x compared to loop-based ProfileRead
//
// 🎯 Best Practices:
// ✅ Always check for errors in the iterator
// ✅ Use meaningful Swamp naming conventions for easy debugging
// ✅ Consider filtering in the iterator to process only relevant profiles
// ✅ Store results in appropriate data structures (map, slice, etc.)
// ✅ Log missing profiles for monitoring and alerting
//
// ⚠️ Important Notes:
// - The model parameter is used as a template - a new instance is created for each Swamp
// - The iterator is called in the same order as swampNames
// - If a Swamp doesn't exist, the iterator receives an error, not a nil model
// - Empty list of swampNames will return an error immediately
// - Iterator function must not be nil
//
// 🚀 Performance Tips:
// - Batch size of 50-200 Swamps works well in practice
// - For very large batches (1000+), consider splitting into smaller chunks
// - Network latency is the bottleneck - batching helps most with remote servers
// - Consider using context with timeout for large batch operations

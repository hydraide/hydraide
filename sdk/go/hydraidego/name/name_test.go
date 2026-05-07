package name

import (
	"crypto/rand"
	"math/big"
	"path/filepath"
	"testing"
)

const (
	testDataCount = 100000
	allFolders    = 100
)

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	return string(result)
}

func TestGetFolderNumberDistribution(t *testing.T) {
	folderCounts := make(map[uint64]int)

	for i := 0; i < testDataCount; i++ {
		// Generate random Sanctuary, Realm, and Swamp names
		sanctuary := randomString(8)
		realm := randomString(8)
		swamp := randomString(8)

		// Construct a new Name
		n := New().Sanctuary(sanctuary).Realm(realm).Swamp(swamp)

		// Determine the folder number
		folder := n.GetIslandID(allFolders)

		// Increment the counter for the corresponding server
		folderCounts[folder]++
	}

	// Log the number of entries per folder
	for folder, count := range folderCounts {
		t.Logf("Server %d: %d entries", folder, count)
	}

	// Check if the distribution is reasonably even.
	// 100k random keys hashed into 100 buckets has a per-bucket stddev of
	// ~sqrt(1000) which is ~32. The maximum of 100 such samples sits 3 to 4
	// stddev above the mean, so the realistic worst-case bucket deviates by
	// ~120 to 150. A 10% threshold flagged this as "broken" on roughly 1 in
	// 5 runs even though the hash is fine. 20% keeps the test stable while
	// still catching a real distribution bug (a broken hash would cluster
	// far worse).
	expectedPerServer := testDataCount / allFolders
	threshold := expectedPerServer / 5 // Allow 20% deviation

	for server, count := range folderCounts {
		if count < expectedPerServer-threshold || count > expectedPerServer+threshold {
			t.Errorf("Server %d received too few or too many entries: %d", server, count)
		}
	}
}

// BenchmarkName_Load measures the performance of reconstructing a Name
// from a canonical path string (e.g. "users/johndoe/info").
// goos: linux
// goarch: amd64
// pkg: github.com/trendizz/hydra-spine/hydra/name
// cpu: AMD Ryzen 9 5950X 16-Core Processor
// BenchmarkName_LoadFromCanonicalForm
// BenchmarkName_Load-32    	 3671059	       320.5 ns/op
func BenchmarkName_Load(b *testing.B) {
	canonicalForm := filepath.Join("users", "johndoe", "info")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Load(canonicalForm)
	}
}

// BenchmarkName_Get measures the performance of the Get() method,
// which returns the canonical path representation of a Name instance
// in the format: "sanctuary/realm/swamp".
//
// This method is lightweight and frequently used internally by the SDK
// for tasks like logging, routing, and debugging.
//
// Example output: "users/johndoe/info"
//
// goos: linux
// goarch: amd64
// pkg: github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name
// cpu: AMD Ryzen Threadripper 2950X 16-Core Processor
// BenchmarkName_Get
// BenchmarkName_Get-32    	1000000000	         0.5228 ns/op
// PASS
func BenchmarkName_Get(b *testing.B) {
	nameObj := New().Sanctuary("users").Realm("johndoe").Swamp("info")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nameObj.Get()
	}
}

// BenchmarkName_Add measures the performance of constructing a full Name
// by chaining Sanctuary(), Realm(), and Swamp() calls.
//
// This benchmark reflects how efficiently the SDK can create hierarchical
// name representations, which are the foundation of most operations in HydrAIDE.
//
// The resulting structure is used internally for data addressing and routing.
//
// Example: "users/johndoe/info"
//
// goos: linux
// goarch: amd64
// pkg: github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name
// cpu: AMD Ryzen Threadripper 2950X 16-Core Processor
// BenchmarkName_Add
// BenchmarkName_Add-32    	25031070	        41.09 ns/op
// PASS
func BenchmarkName_Add(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		New().Sanctuary("users").Realm("johndoe").Swamp("info")
	}
}

// BenchmarkGetFolderNumber measures the performance of computing the
// folder assignment for a given Name using the GetIslandID() method.
//
// This method uses xxhash to deterministically assign a Name to one of N folders,
// enabling stateless, distributed routing logic inside the SDK.
//
// The result is a 1-based folder number (e.g. 1–1000), based on the full path:
// "BenchmarkSanctuary/BenchmarkRealm/BenchmarkSwamp".
//
// goos: linux
// goarch: amd64
// pkg: github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name
// cpu: AMD Ryzen Threadripper 2950X 16-Core Processor
// BenchmarkGetFolderNumber
// BenchmarkGetFolderNumber-32    	76946376	        15.19 ns/op
// PASS
func BenchmarkGetFolderNumber(b *testing.B) {
	sanctuary := "BenchmarkSanctuary"
	realm := "BenchmarkRealm"
	swamp := "BenchmarkSwamp"
	n := New().Sanctuary(sanctuary).Realm(realm).Swamp(swamp)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = n.GetIslandID(allFolders)
	}
}

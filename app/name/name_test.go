package name

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestFolderDistribution(t *testing.T) {
	const (
		totalItems       = 1000000
		totalServers     = 1000
		allowedDeviation = 0.2 // 10%
	)

	// Map of server slot → count
	distribution := make(map[uint16]int)

	for i := 0; i < totalItems; i++ {
		n := New().
			Sanctuary("test").
			Realm("bucket").
			Swamp(fmt.Sprintf("swamp-%d", i))

		server := n.GetFolderNumber(totalServers)
		distribution[server]++
	}

	expectedPerServer := float64(totalItems) / float64(totalServers)
	minAllowed := expectedPerServer * (1 - allowedDeviation)
	maxAllowed := expectedPerServer * (1 + allowedDeviation)

	// Check each slot
	for server, count := range distribution {
		if float64(count) < minAllowed || float64(count) > maxAllowed {
			t.Errorf("Server %d has %d items, expected %.0f ± %.0f (%.1f%% tolerance)",
				server, count, expectedPerServer, expectedPerServer*allowedDeviation, allowedDeviation*100)
		}
	}

	t.Logf("✅ Distribution test passed with %d items over %d servers", totalItems, totalServers)
}

func TestGetFullHashPath(t *testing.T) {

	// Parameters for the test
	rootPath := "/hydraide/data"
	depth := 1
	maxFoldersPerLevel := 1000

	// First Name instance
	name1 := New().
		Sanctuary("Sanctuary1").
		Realm("RealmA").
		Swamp("SwampX")

	// Second Name instance with the same values
	name2 := New().
		Sanctuary("Sanctuary1").
		Realm("RealmA").
		Swamp("SwampX")

	// Third Name instance with different values
	name3 := New().
		Sanctuary("Sanctuary2").
		Realm("RealmB").
		Swamp("SwampY")

	// Test: Do identical inputs generate the same hash path?
	hashPath1 := name1.GetFullHashPath(rootPath, 10, depth, maxFoldersPerLevel)
	hashPath2 := name2.GetFullHashPath(rootPath, 10, depth, maxFoldersPerLevel)

	fmt.Println(hashPath1)

	if hashPath1 != hashPath2 {
		t.Errorf("Hash path mismatch for identical names: %s != %s", hashPath1, hashPath2)
	}

	// Test: Do different inputs generate different hash paths?
	hashPath3 := name3.GetFullHashPath(rootPath, 10, depth, maxFoldersPerLevel)
	if hashPath1 == hashPath3 {
		t.Errorf("Hash path collision: %s == %s", hashPath1, hashPath3)
	}

	// Output for easier verification
	fmt.Println("Hash path 1:", hashPath1)
	fmt.Println("Hash path 2:", hashPath2)
	fmt.Println("Hash path 3:", hashPath3)
}

// goos: linux
// goarch: amd64
// cpu: AMD Ryzen 9 5950X 16-Core Processor
// BenchmarkName_Compare
// BenchmarkName_Compare-32    	64480774	        18.77 ns/op
// NEW BenchmarkName_Compare-32    	11431681	       100.4 ns/op
// Newer BenchmarkName_Compare-32    	150973188	         7.443 ns/op
// Newest BenchmarkName_Compare-32    	98860936	        10.95 ns/op
func BenchmarkName_Compare(b *testing.B) {

	swampName := New().Sanctuary("users").Realm("petergebri").Swamp("info")
	pattern := New().Sanctuary("users").Realm("*").Swamp("info")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		swampName.ComparePattern(pattern)
	}

}

// goos: linux
// goarch: amd64
// cpu: AMD Ryzen 9 5950X 16-Core Processor
// BenchmarkName_LoadFromCanonicalForm
// NEW BenchmarkName_Load-32    	 1630828	       748.7 ns/op
// Newer BenchmarkName_Load-32    	 3155320	       373.6 ns/op
// Newest BenchmarkName_Load-32    	 3427303	       336.8 ns/op
func BenchmarkName_Load(b *testing.B) {

	canonicalForm := filepath.Join("users", "petergebri", "info")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Load(canonicalForm)
	}

}

// goos: linux
// goarch: amd64
// cpu: AMD Ryzen 9 5950X 16-Core Processor
// BenchmarkName_GetCanonicalForm
// NEW BenchmarkName_Get-32    	69597764	        16.46 ns/op
// Newer BenchmarkName_Get-32    	617518238	         1.919 ns/op
// Newest BenchmarkName_Get-32    	1000000000	         0.5444 ns/op
func BenchmarkName_Get(b *testing.B) {

	nameObj := New().Sanctuary("users").Realm("petergebri").Swamp("info")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nameObj.Get()
	}

}

// goos: linux
// goarch: amd64
// pkg: github.com/trendizz/hydra-spine/hydra/name
// cpu: AMD Ryzen 9 5950X 16-Core Processor
// BenchmarkName_Add
// BenchmarkName_Add-32    	 2036251	       592.8 ns/op
// BenchmarkName_Add-32    	19829251	        61.39 ns/op
// Newest BenchmarkName_Add-32    	27526341	        40.42 ns/op
func BenchmarkName_Add(b *testing.B) {

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		New().Sanctuary("users").Realm("petergebri").Swamp("info")
	}

}

// goos: linux
// goarch: amd64
// cpu: AMD Ryzen Threadripper 2950X 16-Core Processor
// BenchmarkName_GetFullHashPath
// BenchmarkName_GetFullHashPath-32    	69819813	        17.16 ns/op
func BenchmarkName_GetFullHashPath(b *testing.B) {

	sanctuary := "BenchmarkSanctuary"
	realm := "BenchmarkRealm"
	swamp := "BenchmarkSwamp"

	n := New().Sanctuary(sanctuary).Realm(realm).Swamp(swamp)

	rootPath := "/hydraide/data"
	depth := 3
	maxFoldersPerLevel := 5000

	b.ResetTimer() // Elindítjuk az időmérést

	for i := 0; i < b.N; i++ {
		n.GetFullHashPath(rootPath, 10, depth, maxFoldersPerLevel) // Meghívjuk a funkciót
	}

}

package explorer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
)

// TestGenerateTestData creates a realistic test dataset that can be used
// to manually test the explore CLI command. Run with:
//
//	go test ./app/server/explorer/ -run TestGenerateTestData -v
//
// Then test the CLI:
//
//	go run ./app/hydraidectl/main.go explore --data-path /tmp/hydraide-explore-test
func TestGenerateTestData(t *testing.T) {
	if os.Getenv("GENERATE_TESTDATA") != "1" {
		t.Skip("Set GENERATE_TESTDATA=1 to generate test data")
	}

	dataRoot := "/tmp/hydraide-explore-test"
	os.RemoveAll(dataRoot)

	counter := 0
	create := func(island int, name string, entries int) {
		counter++
		hash := fmt.Sprintf("%06x", counter)
		dir := filepath.Join(dataRoot, fmt.Sprintf("%d", island), hash[:2])
		os.MkdirAll(dir, 0755)
		fp := filepath.Join(dir, hash+".hyd")

		w, err := v2.NewFileWriterWithName(fp, v2.DefaultMaxBlockSize, name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		for i := 0; i < entries; i++ {
			w.WriteEntry(v2.Entry{
				Operation: v2.OpInsert,
				Key:       fmt.Sprintf("key-%06d", i),
				Data:      []byte(fmt.Sprintf(`{"id":%d,"name":"item-%d","value":%d}`, i, i, i*100)),
			})
		}
		w.Close()
	}

	// users sanctuary — 3 realms, many swamps
	for _, name := range []string{"alice", "bob", "charlie", "diana", "eve", "frank", "grace", "henry"} {
		create(1, "users/profiles/"+name, 50+len(name)*10)
	}
	for _, name := range []string{"alice", "bob", "charlie", "diana"} {
		create(1, "users/sessions/"+name, 10+len(name)*5)
	}
	for _, name := range []string{"alice", "bob"} {
		create(2, "users/preferences/"+name, 5)
	}

	// products sanctuary — 2 realms
	for i := 1; i <= 20; i++ {
		create(3, fmt.Sprintf("products/catalog/item-%04d", i), 100+i*10)
	}
	for i := 1; i <= 10; i++ {
		create(3, fmt.Sprintf("products/reviews/item-%04d", i), 50+i*20)
	}

	// analytics sanctuary — 1 realm, few big swamps
	for _, name := range []string{"page-views", "click-events", "search-queries", "api-calls", "error-logs"} {
		create(5, "analytics/events/"+name, 5000)
	}

	// sessions sanctuary — 1 realm, many tiny swamps
	for i := 1; i <= 50; i++ {
		create(4, fmt.Sprintf("sessions/active/session-%06d", i), 3)
	}

	// orders sanctuary — 2 realms
	for i := 1; i <= 15; i++ {
		create(6, fmt.Sprintf("orders/pending/order-%06d", i), 20)
	}
	for i := 1; i <= 30; i++ {
		create(6, fmt.Sprintf("orders/completed/order-%06d", i), 25)
	}

	t.Logf("Generated test data at: %s", dataRoot)
	t.Log("Test with:")
	t.Logf("  go run ./app/hydraidectl/main.go explore --data-path %s", dataRoot)
	t.Logf("  go run ./app/hydraidectl/main.go explore --data-path %s -s users", dataRoot)
	t.Logf("  go run ./app/hydraidectl/main.go explore --data-path %s -s users -r profiles", dataRoot)
	t.Logf("  go run ./app/hydraidectl/main.go explore --data-path %s -s users -r profiles -w alice", dataRoot)
}

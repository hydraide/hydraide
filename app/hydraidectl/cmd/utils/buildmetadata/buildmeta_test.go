package buildmeta

import (
	"testing"
)

func TestNew(t *testing.T) {
	bm, err := New()
	if err != nil {
		t.Errorf("New() returned an error: %v", err)
	}

	if bm == nil {
		t.Errorf("New() returned nil")
	}
}

func TestBuildMetadata_Get(t *testing.T) {
	bm, err := New()
	if err != nil {
		t.Errorf("New() returned an error: %v", err)
	}

	// Test getting a non-existent key
	_, err = bm.Get("non-existent-key")
	if err == nil {
		t.Errorf("Get() did not return an error for non-existent key")
	}

	// Test getting an existing key
	err = bm.Update("existing-key", "existing-value")
	if err != nil {
		t.Errorf("Update() returned an error: %v", err)
	}

	value, err := bm.Get("existing-key")
	if err != nil {
		t.Errorf("Get() returned an error: %v", err)
	}

	if value != "existing-value" {
		t.Errorf("Get() returned incorrect value: %s", value)
	}
}

func TestBuildMetadata_Update(t *testing.T) {
	bm, err := New()
	if err != nil {
		t.Errorf("New() returned an error: %v", err)
	}

	// Test updating a key
	err = bm.Update("key", "value")
	if err != nil {
		t.Errorf("Update() returned an error: %v", err)
	}

	// Test updating an existing key
	err = bm.Update("key", "new-value")
	if err != nil {
		t.Errorf("Update() returned an error: %v", err)
	}

	value, err := bm.Get("key")
	if err != nil {
		t.Errorf("Get() returned an error: %v", err)
	}

	if value != "new-value" {
		t.Errorf("Update() did not update the value correctly: %s", value)
	}
}

func TestBuildMetadata_Delete(t *testing.T) {
	bm, err := New()
	if err != nil {
		t.Errorf("New() returned an error: %v", err)
	}

	// Test deleting a non-existent key
	err = bm.Delete("non-existent-key")
	if err != nil {
		t.Errorf("Delete() returned an error for non-existent key: %v", err)
	}

	// Test deleting an existing key
	err = bm.Update("existing-key", "existing-value")
	if err != nil {
		t.Errorf("Update() returned an error: %v", err)
	}

	err = bm.Delete("existing-key")
	if err != nil {
		t.Errorf("Delete() returned an error: %v", err)
	}

	_, err = bm.Get("existing-key")
	if err == nil {
		t.Errorf("Get() did not return an error for deleted key")
	}
}

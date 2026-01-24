package filesystem

import (
	"bytes"
	"github.com/hydraide/hydraide/app/core/compressor"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	testRootFolder = "./test_data"
	maxDepth       = 3
)

// setupTestEnvironment prepares the test directory structure before each test run.
func setupTestEnvironment() error {
	// Create base folder structure for tests
	return os.MkdirAll(testRootFolder, os.ModePerm)
}

// cleanupTestEnvironment removes the entire test environment after each test.
func cleanupTestEnvironment() {
	// Delete the test root folder and all its contents
	if err := os.RemoveAll(testRootFolder); err != nil {
		panic(err)
	}
}

// TestCreateFolder tests the CreateFolder function for basic behavior and error handling.
func TestCreateFolder(t *testing.T) {
	fs := New()

	// Set up test environment
	err := setupTestEnvironment()
	if err != nil {
		t.Fatalf("Failed to set up test environment: %v", err)
	}
	defer cleanupTestEnvironment()

	// Test 1: Create a folder that doesn't exist
	testFolder := filepath.Join(testRootFolder, "folder1")
	err = fs.CreateFolder(testFolder)
	if err != nil {
		t.Errorf("Failed to create folder: %v", err)
	}
	// Verify that the folder actually exists
	if _, err := os.Stat(testFolder); os.IsNotExist(err) {
		t.Errorf("Expected folder %s to exist, but it does not", testFolder)
	}

	// Test 2: Create a folder that already exists
	err = fs.CreateFolder(testFolder)
	if err != nil {
		t.Errorf("Failed to create an already existing folder: %v", err)
	}

	// Test 3: Try creating a folder with an empty path
	err = fs.CreateFolder("")
	if err == nil {
		t.Errorf("Expected error for empty folder path, but got nil")
	}
}

// TestConcurrentCreateAndDelete tests concurrent use of CreateFolder and DeleteFolder.
func TestConcurrentCreateAndDelete(t *testing.T) {
	fs := New()

	// Set up test environment
	err := setupTestEnvironment()
	if err != nil {
		t.Fatalf("Failed to set up test environment: %v", err)
	}
	defer cleanupTestEnvironment()

	// Folder path for concurrent access test
	testFolder := filepath.Join(testRootFolder, "concurrent_test")

	const goroutineCount = 10 // Number of concurrent goroutines

	// Phase 1: Concurrent folder creation (all goroutines try to create the same folder)
	var wg sync.WaitGroup
	wg.Add(goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		go func(id int) {
			defer wg.Done()
			// Random sleep to increase race condition likelihood
			time.Sleep(time.Duration(id) * 10 * time.Millisecond)
			err := fs.CreateFolder(testFolder)
			if err != nil {
				t.Errorf("Goroutine %d: failed to create folder: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify folder exists after creation phase
	if _, err := os.Stat(testFolder); os.IsNotExist(err) {
		t.Errorf("Expected folder %s to exist after creation phase", testFolder)
	}

	// Phase 2: Concurrent folder deletion (all goroutines try to delete the same folder)
	wg.Add(goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		go func(id int) {
			defer wg.Done()
			// Random sleep to increase race condition likelihood
			time.Sleep(time.Duration(id) * 10 * time.Millisecond)
			err := fs.DeleteFolder(testFolder, maxDepth)
			if err != nil && !os.IsNotExist(err) {
				t.Errorf("Goroutine %d: failed to delete folder: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// Final check: folder should not exist
	if _, err := os.Stat(testFolder); !os.IsNotExist(err) {
		t.Errorf("Expected folder %s to be deleted, but it still exists", testFolder)
	}
}

// TestSaveGetDeleteFile tests the SaveFile, GetFile, and DeleteFile functionalities end-to-end.
func TestSaveGetDeleteFile(t *testing.T) {
	fs := New()

	// Set up the test environment
	err := setupTestEnvironment()
	if err != nil {
		t.Fatalf("Failed to set up test environment: %v", err)
	}
	defer cleanupTestEnvironment()

	// Prepare test data
	filePath := filepath.Join(testRootFolder, "test_file.dat")
	initialContent := [][]byte{
		[]byte("block1"),
		[]byte("block2"),
		[]byte("block3"),
	}

	// Test 1: Save file with initial content (append = false)
	err = fs.SaveFile(filePath, initialContent, false)
	if err != nil {
		t.Fatalf("Failed to save file with initial content: %v", err)
	}

	// Ensure file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("Expected file %s to be created, but it does not exist", filePath)
	}

	// Test 2: Read file and verify content
	readContent, err := fs.GetFile(filePath)
	if err != nil {
		t.Fatalf("Failed to get file content: %v", err)
	}
	if !compareContent(initialContent, readContent) {
		t.Errorf("Content mismatch: expected %v, got %v", initialContent, readContent)
	}

	// Test 3: Overwrite file with new content (append = false)
	newContent := [][]byte{
		[]byte("new_block1"),
		[]byte("new_block2"),
	}
	err = fs.SaveFile(filePath, newContent, false)
	if err != nil {
		t.Fatalf("Failed to overwrite file content: %v", err)
	}

	// Verify content after overwrite
	readContent, err = fs.GetFile(filePath)
	if err != nil {
		t.Fatalf("Failed to get file content after overwrite: %v", err)
	}
	if !compareContent(newContent, readContent) {
		t.Errorf("Content mismatch after overwrite: expected %v, got %v", newContent, readContent)
	}

	// Test 4: Append new blocks (append = true)
	appendContent := [][]byte{
		[]byte("appended_block1"),
		[]byte("appended_block2"),
	}
	err = fs.SaveFile(filePath, appendContent, true)
	if err != nil {
		t.Fatalf("Failed to append to file: %v", err)
	}

	// Verify final content after append
	expectedContent := append(newContent, appendContent...)
	readContent, err = fs.GetFile(filePath)
	if err != nil {
		t.Fatalf("Failed to get file content after append: %v", err)
	}
	if !compareContent(expectedContent, readContent) {
		t.Errorf("Content mismatch after append: expected %v, got %v", expectedContent, readContent)
	}

	// Test 5: Delete file
	err = fs.DeleteFile(filePath)
	if err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}

	// Ensure file is deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("Expected file %s to be deleted, but it still exists", filePath)
	}
}

// compareContent compares two [][]byte slices for deep equality.
func compareContent(content1, content2 [][]byte) bool {
	if len(content1) != len(content2) {
		return false
	}
	for i := range content1 {
		if !bytes.Equal(content1[i], content2[i]) {
			return false
		}
	}
	return true
}

// generateTestContent returns a simple generated set of binary blocks for testing.
func generateTestContent(blocks int, baseValue int) [][]byte {
	content := make([][]byte, blocks)
	for i := 0; i < blocks; i++ {
		content[i] = []byte{byte(baseValue), byte(i)}
	}
	return content
}

// TestGetFileSize verifies the correctness of the GetFileSize function,
// including compressed size accuracy and error handling for edge cases.
func TestGetFileSize(t *testing.T) {

	compressorInterface := compressor.New(compressor.Snappy)
	fs := New()

	// Set up test environment
	err := setupTestEnvironment()
	if err != nil {
		t.Fatalf("Failed to set up test environment: %v", err)
	}
	defer cleanupTestEnvironment()

	// Create a test folder
	testFolder := filepath.Join(testRootFolder, "file_size_test")
	err = fs.CreateFolder(testFolder)
	if err != nil {
		t.Fatalf("Failed to create test folder: %v", err)
	}

	// 1. Test: Create files with known content and verify their compressed size
	fileContents := map[string][][]byte{
		"file1.dat": generateTestContent(10, 1), // 10 blocks
		"file2.dat": generateTestContent(20, 2), // 20 blocks
		"file3.dat": generateTestContent(5, 3),  // 5 blocks
	}

	for fileName, content := range fileContents {
		// Save file
		filePath := filepath.Join(testFolder, fileName)
		err = fs.SaveFile(filePath, content, false)
		if err != nil {
			t.Fatalf("Failed to save file %s: %v", fileName, err)
		}

		// Compute expected compressed size
		compressedContent, err := compressorInterface.Compress(flattenContent(content))
		if err != nil {
			t.Fatalf("Failed to compress content for %s: %v", fileName, err)
		}
		expectedSize := int64(len(compressedContent))

		// Get actual file size
		size, err := fs.GetFileSize(filePath)
		if err != nil {
			t.Errorf("Failed to get file size for %s: %v", fileName, err)
		}
		if size != expectedSize {
			t.Errorf("File size mismatch for %s: expected %d bytes (compressed), got %d bytes", fileName, expectedSize, size)
		}
	}

	// 2. Test: Create and check the size of an empty file
	emptyFilePath := filepath.Join(testFolder, "empty_file.dat")
	_, err = os.Create(emptyFilePath)
	if err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}

	size, err := fs.GetFileSize(emptyFilePath)
	if err != nil {
		t.Errorf("Failed to get file size for empty file: %v", err)
	}
	if size != 0 {
		t.Errorf("File size mismatch for empty file: expected 0 bytes, got %d bytes", size)
	}

	// 3. Test: Try to get size of a non-existent file
	nonExistentFile := filepath.Join(testFolder, "non_existent_file.dat")
	_, err = fs.GetFileSize(nonExistentFile)
	if err == nil {
		t.Errorf("Expected error for non-existent file %s, but got nil", nonExistentFile)
	}
}

// flattenContent prepares [][]byte data into a single binary slice,
// encoding length headers for compression or write operations.
func flattenContent(content [][]byte) []byte {
	var flattened []byte
	for _, part := range content {
		flattened = append(flattened, encodeBinaryLength(part)...)
		flattened = append(flattened, part...)
	}
	return flattened
}

// TestDeleteAllFiles tests the DeleteAllFiles method across multiple edge cases.
func TestDeleteAllFiles(t *testing.T) {
	fs := New()

	// Set up the test environment
	err := setupTestEnvironment()
	if err != nil {
		t.Fatalf("Failed to set up test environment: %v", err)
	}
	defer cleanupTestEnvironment()

	// Create test folder
	testFolder := filepath.Join(testRootFolder, "delete_all_files_test")
	err = fs.CreateFolder(testFolder)
	if err != nil {
		t.Fatalf("Failed to create test folder: %v", err)
	}

	// 1. Test: Delete all files in a folder
	files := []string{"file1.txt", "file2.log", "file3.dat", "file4.bin", "file5.tmp"}
	for _, file := range files {
		filePath := filepath.Join(testFolder, file)
		err := os.WriteFile(filePath, []byte("test content"), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", filePath, err)
		}
	}

	err = fs.DeleteAllFiles(testFolder)
	if err != nil {
		t.Errorf("Failed to delete all files in folder %s: %v", testFolder, err)
	}

	for _, file := range files {
		filePath := filepath.Join(testFolder, file)
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Errorf("Expected file %s to be deleted, but it still exists", filePath)
		}
	}

	// 2. Test: Empty folder handling
	emptyFolder := filepath.Join(testFolder, "empty_folder")
	err = fs.CreateFolder(emptyFolder)
	if err != nil {
		t.Fatalf("Failed to create empty test folder: %v", err)
	}

	err = fs.DeleteAllFiles(emptyFolder)
	if err != nil {
		t.Errorf("Failed to delete all files in empty folder %s: %v", emptyFolder, err)
	}

	if _, err := os.Stat(emptyFolder); os.IsNotExist(err) {
		t.Errorf("Expected empty folder %s to exist, but it was deleted", emptyFolder)
	}

	// 3. Test: Subfolder preservation
	subFolder := filepath.Join(testFolder, "subfolder")
	err = fs.CreateFolder(subFolder)
	if err != nil {
		t.Fatalf("Failed to create subfolder: %v", err)
	}
	subFile := filepath.Join(subFolder, "subfile.txt")
	err = os.WriteFile(subFile, []byte("subfolder content"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create subfile %s: %v", subFile, err)
	}

	rootFile := filepath.Join(testFolder, "rootfile.txt")
	err = os.WriteFile(rootFile, []byte("root file content"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create root file %s: %v", rootFile, err)
	}

	err = fs.DeleteAllFiles(testFolder)
	if err != nil {
		t.Errorf("Failed to delete all files in folder %s: %v", testFolder, err)
	}

	if _, err := os.Stat(rootFile); !os.IsNotExist(err) {
		t.Errorf("Expected root file %s to be deleted, but it still exists", rootFile)
	}
	if _, err := os.Stat(subFolder); os.IsNotExist(err) {
		t.Errorf("Expected subfolder %s to exist, but it was deleted", subFolder)
	}
	if _, err := os.Stat(subFile); os.IsNotExist(err) {
		t.Errorf("Expected file %s inside subfolder to exist, but it was deleted", subFile)
	}

	// 4. Test: Non-existent folder handling
	nonExistentFolder := filepath.Join(testRootFolder, "non_existent_folder")
	err = fs.DeleteAllFiles(nonExistentFolder)
	if err != nil {
		t.Errorf("Expected no error for non-existent folder %s, but got: %v", nonExistentFolder, err)
	}

	// 5. Test: Delete files with various extensions
	fileFormats := []string{"file1.txt", "file2.log", "file3.csv", "file4.json", "file5.xml"}
	for _, file := range fileFormats {
		filePath := filepath.Join(testFolder, file)
		err := os.WriteFile(filePath, []byte("format test content"), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", filePath, err)
		}
	}

	err = fs.DeleteAllFiles(testFolder)
	if err != nil {
		t.Errorf("Failed to delete all files in folder %s: %v", testFolder, err)
	}

	for _, file := range fileFormats {
		filePath := filepath.Join(testFolder, file)
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Errorf("Expected file %s to be deleted, but it still exists", filePath)
		}
	}
}

package filesystem

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// minimalk fs implementation
func newFSForTest() *fileSystemImpl {
	return &fileSystemImpl{
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}
}

func writeTempFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(content), mode); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return full
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(b)
}

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("mismatch: got=%v want=%v", got, want)
	}
}

func assertNotExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected %s to not exist", p)
	}
}

func TestMoveFile_RenameSuccess_SameDir(t *testing.T) {
	fs := newFSForTest()
	ctx := context.Background()

	tmp := t.TempDir()
	src := writeTempFile(t, tmp, "client.crt", "CERTDATA", 0o644)
	dst := filepath.Join(tmp, "moved", "client.crt")

	err := fs.MoveFile(ctx, src, dst)
	assertNoErr(t, err)

	assertNotExist(t, src)
	assertEq(t, readFile(t, dst), "CERTDATA")
	info, err := os.Stat(dst)
	assertNoErr(t, err)

	if runtime.GOOS != "windows" {
		if info.Mode()&0o777 != 0o644 {
			t.Fatalf("unexpected mode: %v", info.Mode()&0o777)
		}
	}
}

func TestCopyThenReplace_Basic(t *testing.T) {

	fs := newFSForTest()
	ctx := context.Background()

	tmp := t.TempDir()
	src := writeTempFile(t, tmp, "client.key", "KEYDATA", 0o600)

	dstDir := filepath.Join(tmp, "dest")
	dst := filepath.Join(dstDir, "client.key")

	err := fs.copyThenReplace(ctx, src, dst)
	assertNoErr(t, err)

	assertNotExist(t, src)
	assertEq(t, readFile(t, dst), "KEYDATA")

	src2 := writeTempFile(t, tmp, "client.key.v2", "KEYDATA_V2", 0o600)
	err = fs.copyThenReplace(ctx, src2, dst)
	assertNoErr(t, err)

	assertNotExist(t, src2)
	assertEq(t, readFile(t, dst), "KEYDATA_V2")
}

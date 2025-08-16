// servicehelper/service_linux_test.go
package servicehelper

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerateServiceFile_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only unit test")
	}

	tmp := t.TempDir()
	basePath := filepath.Join(tmp, "app")
	_ = os.MkdirAll(basePath, 0o755)

	// dummy bin
	bin := filepath.Join(basePath, LINUX_MAC_BINARY_NAME)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	mock := &MockRunner{
		Script: map[string]mockResp{
			"systemctl daemon-reload":                   {},
			"systemctl enable hydraserver-test.service": {},
			"systemctl start hydraserver-test.service":  {},
		},
	}

	sysdDir := filepath.Join(tmp, "systemd")
	_ = os.MkdirAll(sysdDir, 0o755)

	s := newWithDeps(deps{
		runner: mock,
		paths:  FSPaths{SystemdDir: sysdDir},
	})

	if err := s.generateSystemdService("test", basePath); err != nil {
		t.Fatalf("generateSystemdService error: %v", err)
	}

	unitPath := filepath.Join(sysdDir, "hydraserver-test.service")
	b, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(b)
	if !strings.Contains(content, "ExecStart="+bin) {
		t.Errorf("unit missing ExecStart with bin path: %s", content)
	}
	if !strings.Contains(content, "WorkingDirectory="+basePath) {
		t.Errorf("unit missing WorkingDirectory")
	}

	wantCalls := []string{
		"systemctl daemon-reload",
		"systemctl enable hydraserver-test.service",
		"systemctl start hydraserver-test.service",
	}
	if len(mock.Calls) != len(wantCalls) {
		t.Fatalf("calls mismatch: got %v, want %v", mock.Calls, wantCalls)
	}
	for i, c := range wantCalls {
		if mock.Calls[i] != c {
			t.Errorf("call[%d]=%q, want %q", i, mock.Calls[i], c)
		}
	}
}

func TestServiceExists_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}

	tmp := t.TempDir()
	sysdDir := filepath.Join(tmp, "systemd")
	_ = os.MkdirAll(sysdDir, 0o755)

	s := newWithDeps(deps{
		runner: &MockRunner{Script: map[string]mockResp{}},
		paths:  FSPaths{SystemdDir: sysdDir},
	})

	// not exists
	exists, err := s.ServiceExists("x")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected not exists")
	}

	// create file -> exists
	unit := filepath.Join(sysdDir, "hydraserver-x.service")
	if err := os.WriteFile(unit, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	exists, err = s.ServiceExists("x")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected exists")
	}
}

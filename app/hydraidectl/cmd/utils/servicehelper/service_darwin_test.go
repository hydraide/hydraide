//go:build darwin

// servicehelper/service_darwin_test.go
package servicehelper

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerateLaunchdService_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}

	tmp := t.TempDir()
	basePath := filepath.Join(tmp, "app")
	_ = os.MkdirAll(basePath, 0o755)

	bin := filepath.Join(basePath, LINUX_MAC_BINARY_NAME)
	if err := os.WriteFile(bin, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	launchDir := filepath.Join(tmp, "LaunchDaemons")
	_ = os.MkdirAll(launchDir, 0o755)

	label := "com.hydraide.hydraserver-test"
	plist := filepath.Join(launchDir, label+".plist")

	mock := &MockRunner{
		Script: map[string]mockResp{
			// jogosultságok a plist-re
			"chown root:wheel " + plist: {},
			"chmod 644 " + plist:        {},

			// launchctl flow
			"launchctl bootstrap system " + plist:    {},
			"launchctl enable system/" + label:       {},
			"launchctl kickstart -k system/" + label: {},
		},
	}

	s := newWithDeps(deps{
		runner: mock,
		paths:  FSPaths{LaunchDaemonsDir: launchDir},
	})

	if err := s.generateLaunchdService("test", basePath); err != nil {
		t.Fatalf("generateLaunchdService error: %v", err)
	}

	b, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	if !strings.Contains(content, "<key>Label</key><string>"+label+"</string>") {
		t.Errorf("plist label missing")
	}
	if !strings.Contains(content, "<string>"+bin+"</string>") {
		t.Errorf("plist missing ProgramArguments with bin")
	}
	if !strings.Contains(content, "<key>WorkingDirectory</key><string>"+basePath+"</string>") {
		t.Errorf("plist missing WorkingDirectory")
	}
}

func TestServiceExists_Darwin(t *testing.T) {
	tmp := t.TempDir()
	launchDir := filepath.Join(tmp, "LaunchDaemons")
	_ = os.MkdirAll(launchDir, 0o755)

	mock := &MockRunner{
		Script: map[string]mockResp{
			"launchctl print system/com.hydraide.hydraserver-x": {Out: "ok"},
		},
	}

	s := newWithDeps(deps{
		runner: mock,
		paths:  FSPaths{LaunchDaemonsDir: launchDir},
	})

	// no plist -> false
	exists, err := s.ServiceExists("x")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected false (no plist)")
	}

	// create plist -> true
	plist := filepath.Join(launchDir, "com.hydraide.hydraserver-x.plist")
	if err := os.WriteFile(plist, []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	exists, err = s.ServiceExists("x")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected true")
	}
}

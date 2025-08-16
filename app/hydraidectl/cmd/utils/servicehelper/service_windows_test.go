//go:build windows

// servicehelper/service_windows_test.go
package servicehelper

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateWindowsService_NSSM(t *testing.T) {
	tmp := t.TempDir()
	basePath := filepath.Join(tmp, "app")
	_ = os.MkdirAll(basePath, 0o755)
	bin := filepath.Join(basePath, WINDOWS_BINARY_NAME)
	if err := os.WriteFile(bin, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := map[string]mockResp{
		// nssm telepítve
		"nssm version": {Out: "2.24"},

		// install
		"nssm install hydraserver-test " + bin: {},

		// beállítások
		"nssm set hydraserver-test DisplayName HydrAIDE Service - test":                     {},
		"nssm set hydraserver-test Description HydrAIDE Service Instance: test":             {},
		"nssm set hydraserver-test Start SERVICE_AUTO_START":                                {},
		"nssm set hydraserver-test AppDirectory " + basePath:                                {},
		"nssm set hydraserver-test AppStdout " + filepath.Join(basePath, "logs", "app.log"): {},
		"nssm set hydraserver-test AppStderr " + filepath.Join(basePath, "logs", "app.log"): {},
		"nssm set hydraserver-test AppRotateFiles 1":                                        {},
		"nssm set hydraserver-test AppRotateSeconds 86400":                                  {},
		"nssm set hydraserver-test AppRotateBytes 10485760":                                 {},
	}

	mock := &MockRunner{Script: script}
	s := newWithDeps(deps{runner: mock, paths: FSPaths{}})

	if err := s.generateWindowsNSSMService("test", basePath); err != nil {
		t.Fatalf("generateWindowsNSSMService error: %v", err)
	}

	// (opcionális) hívás-sorrend ellenőrzése
	want := []string{
		"nssm version",
		"nssm install hydraserver-test " + bin,
		"nssm set hydraserver-test DisplayName HydrAIDE Service - test",
		"nssm set hydraserver-test Description HydrAIDE Service Instance: test",
		"nssm set hydraserver-test Start SERVICE_AUTO_START",
		"nssm set hydraserver-test AppDirectory " + basePath,
		"nssm set hydraserver-test AppStdout " + filepath.Join(basePath, "logs", "app.log"),
		"nssm set hydraserver-test AppStderr " + filepath.Join(basePath, "logs", "app.log"),
		"nssm set hydraserver-test AppRotateFiles 1",
		"nssm set hydraserver-test AppRotateSeconds 86400",
		"nssm set hydraserver-test AppRotateBytes 10485760",
	}
	if len(mock.Calls) != len(want) {
		t.Fatalf("calls mismatch: got %v, want %v", mock.Calls, want)
	}
	for i := range want {
		if mock.Calls[i] != want[i] {
			t.Errorf("call[%d]=%q, want %q", i, mock.Calls[i], want[i])
		}
	}
}

func TestServiceExists_Windows_Order(t *testing.T) {
	// 1) nssm status FAIL -> 2) schtasks OK
	mock := &MockRunner{
		Script: map[string]mockResp{
			"nssm status hydraserver-x":         {Err: errors.New("not found")},
			"schtasks /query /tn hydraserver-x": {}, // success -> exists
		},
	}
	s := newWithDeps(deps{runner: mock, paths: FSPaths{}})

	ok, err := s.ServiceExists("x")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected exists via schtasks")
	}
}

func TestGenerateWindowsService_NSSM_InstallsViaWinget(t *testing.T) {
	tmp := t.TempDir()
	basePath := filepath.Join(tmp, "app")
	_ = os.MkdirAll(basePath, 0o755)
	bin := filepath.Join(basePath, WINDOWS_BINARY_NAME)
	if err := os.WriteFile(bin, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	mock := &MockRunner{
		Script: map[string]mockResp{
			// nssm hiányzik -> winget telepít
			"nssm version": {Err: errors.New("not found")},
			"winget install --id=nssm.nssm --source=winget --accept-package-agreements --accept-source-agreements": {},

			// utána megy az install + set
			"nssm install hydraserver-test " + bin:                                              {},
			"nssm set hydraserver-test DisplayName HydrAIDE Service - test":                     {},
			"nssm set hydraserver-test Description HydrAIDE Service Instance: test":             {},
			"nssm set hydraserver-test Start SERVICE_AUTO_START":                                {},
			"nssm set hydraserver-test AppDirectory " + basePath:                                {},
			"nssm set hydraserver-test AppStdout " + filepath.Join(basePath, "logs", "app.log"): {},
			"nssm set hydraserver-test AppStderr " + filepath.Join(basePath, "logs", "app.log"): {},
			"nssm set hydraserver-test AppRotateFiles 1":                                        {},
			"nssm set hydraserver-test AppRotateSeconds 86400":                                  {},
			"nssm set hydraserver-test AppRotateBytes 10485760":                                 {},
		},
	}

	s := newWithDeps(deps{runner: mock, paths: FSPaths{}})
	if err := s.generateWindowsNSSMService("test", basePath); err != nil {
		t.Fatalf("generateWindowsNSSMService (winget path) error: %v", err)
	}
}

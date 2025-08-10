package instancedetector

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

// MockCommandExecutor is a test-only implementation of CommandExecutor.
// It allows us to control the output and errors for the tests.
type MockCommandExecutor struct {
	output []byte
	err    error
}

func (m *MockCommandExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	return m.output, m.err
}

func TestLinuxDetector_ListInstances(t *testing.T) {
	testCases := []struct {
		name              string
		mockExecutor      *MockCommandExecutor
		expectedInstances []Instance
		expectedErr       bool
	}{
		{
			name: "Success with multiple instances",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[{"unit": "hydraserver-dev.service", "load": "loaded", "sub": "running"}, {"unit": "hydraserver-staging.service", "load": "loaded", "sub": "dead"}]`),
				err:    nil,
			},
			expectedInstances: []Instance{
				{Name: "dev", Status: "active"},
				{Name: "staging", Status: "inactive"},
			},
			expectedErr: false,
		},
		{
			name: "Success with no matching instances",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[{"unit": "some-other.service", "load": "loaded", "sub": "running"}]`),
				err:    nil,
			},
			expectedInstances: []Instance{},
			expectedErr:       false,
		},
		{
			name: "Systemctl command fails",
			mockExecutor: &MockCommandExecutor{
				output: nil,
				err:    errors.New("mock command error"),
			},
			expectedInstances: nil,
			expectedErr:       true,
		},
		{
			name: "Invalid JSON output",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`this is not json`),
				err:    nil,
			},
			expectedInstances: nil,
			expectedErr:       true,
		},
		{
			name: "Units in masked or not-found state are ignored",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[{"unit": "hydraserver-dev.service", "load": "loaded", "sub": "running"}, {"unit": "hydraserver-masked.service", "load": "masked", "sub": "dead"}]`),
				err:    nil,
			},
			expectedInstances: []Instance{
				{Name: "dev", Status: "active"},
			},
			expectedErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := &linuxDetector{executor: tc.mockExecutor}
			instances, err := detector.ListInstances(context.Background())

			if tc.expectedErr {
				if err == nil {
					t.Fatal("Expected an error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Did not expect an error, but got: %v", err)
			}

			if !reflect.DeepEqual(instances, tc.expectedInstances) && !(len(instances) == 0 && len(tc.expectedInstances) == 0) {
				t.Errorf("Expected instances: %+v, but got: %+v", tc.expectedInstances, instances)
			}
		})
	}
}

// Test for the normalizeStatus function is still valid and doesn't need mocking.
func TestNormalizeStatus(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"running", "active"},
		{"exited", "inactive"},
		{"dead", "inactive"},
		{"inactive", "inactive"},
		{"failed", "failed"},
		{"activating", "activating"},
		{"deactivating", "deactivating"},
		{"some-other-state", "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeStatus(tc.input)
			if result != tc.expected {
				t.Errorf("For input %s, expected %s, but got %s", tc.input, tc.expected, result)
			}
		})
	}
}

func TestWindowsDetector_ListInstances(t *testing.T) {
	testCases := []struct {
		name              string
		mockExecutor      *MockCommandExecutor
		expectedInstances []Instance
		expectedErr       bool
	}{
		{
			name: "Success with multiple instances",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[{"Name":"hydraserver-dev","Status":"Running"},{"Name":"hydraserver-staging","Status":"Stopped"}]`),
				err:    nil,
			},
			expectedInstances: []Instance{
				{Name: "dev", Status: "active"},
				{Name: "staging", Status: "inactive"},
			},
			expectedErr: false,
		},
		{
			name: "Success with a single instance",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[{"Name":"hydraserver-live","Status":"StartPending"}]`),
				err:    nil,
			},
			expectedInstances: []Instance{
				{Name: "live", Status: "activating"},
			},
			expectedErr: false,
		},
		{
			name: "Success with no matching services",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[]`),
				err:    nil,
			},
			expectedInstances: []Instance{},
			expectedErr:       false,
		},
		{
			name: "Success with empty output (no services found)",
			mockExecutor: &MockCommandExecutor{
				output: []byte(``),
				err:    nil,
			},
			expectedInstances: []Instance{},
			expectedErr:       false,
		},
		{
			name: "Command execution error",
			mockExecutor: &MockCommandExecutor{
				output: []byte(""),
				err:    errors.New("powershell command failed"),
			},
			expectedInstances: nil,
			expectedErr:       true,
		},
		{
			name: "Malformed JSON output",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[{"Name":"hydraserver-dev"}`),
				err:    nil,
			},
			expectedInstances: nil,
			expectedErr:       true,
		},
		{
			name: "Services with non-matching names are filtered",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[{"Name":"hydraserver-dev","Status":"Running"}, {"Name":"some-other-service","Status":"Running"}]`),
				err:    nil,
			},
			expectedInstances: []Instance{
				{Name: "dev", Status: "active"},
			},
			expectedErr: false,
		},
		{
			name: "Multiple status types are handled correctly",
			mockExecutor: &MockCommandExecutor{
				output: []byte(`[{"Name":"hydraserver-s1","Status":"Running"},{"Name":"hydraserver-s2","Status":"Stopped"},{"Name":"hydraserver-s3","Status":"Paused"},{"Name":"hydraserver-s4","Status":"StartPending"},{"Name":"hydraserver-s5","Status":"StopPending"}]`),
				err:    nil,
			},
			expectedInstances: []Instance{
				{Name: "s1", Status: "active"},
				{Name: "s2", Status: "inactive"},
				{Name: "s3", Status: "inactive"},
				{Name: "s4", Status: "activating"},
				{Name: "s5", Status: "deactivating"},
			},
			expectedErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := &windowsDetector{executor: tc.mockExecutor}
			instances, err := detector.ListInstances(context.Background())

			if tc.expectedErr {
				if err == nil {
					t.Fatal("Expected an error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Did not expect an error, but got: %v", err)
			}

			if !reflect.DeepEqual(instances, tc.expectedInstances) {
				t.Errorf("Expected instances: %+v, but got: %+v", tc.expectedInstances, instances)
			}
		})
	}
}

func TestNormalizeWindowsStatus(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Running", "active"},
		{"Stopped", "inactive"},
		{"StartPending", "activating"},
		{"StopPending", "deactivating"},
		{"Paused", "inactive"},
		{"UnknownStatus", "unknown"},
		{"running", "active"},
		{"RUNNING", "active"},
		{"stopped", "inactive"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeWindowsStatus(tc.input)
			if result != tc.expected {
				t.Errorf("For input %s, expected %s, but got %s", tc.input, tc.expected, result)
			}
		})
	}
}

func TestLinuxDetector_GetInstanceStatus(t *testing.T) {
	testCases := []struct {
		name           string
		instanceName   string
		mockExecutor   *MockCommandExecutor
		expectedStatus string
		expectedErr    bool
	}{
		{
			name:         "Success - Active Service",
			instanceName: "dev",
			mockExecutor: &MockCommandExecutor{
				output: []byte("SubState=running"),
				err:    nil,
			},
			expectedStatus: "active",
			expectedErr:    false,
		},
		{
			name:         "Success - Inactive Service",
			instanceName: "staging",
			mockExecutor: &MockCommandExecutor{
				output: []byte("SubState=dead"),
				err:    nil,
			},
			expectedStatus: "inactive",
			expectedErr:    false,
		},
		{
			name:         "Service Not Found",
			instanceName: "nonexistent",
			mockExecutor: &MockCommandExecutor{
				output: []byte("Unit hydraserver-nonexistent.service could not be found."),
				err: &exec.ExitError{
					Stderr:       []byte("Unit hydraserver-nonexistent.service could not be found."),
					ProcessState: nil,
				},
			},
			expectedStatus: "not-found",
			expectedErr:    false,
		},
		{
			name:         "Malformed Output",
			instanceName: "dev",
			mockExecutor: &MockCommandExecutor{
				output: []byte("unexpected output format"),
				err:    nil,
			},
			expectedStatus: "unknown",
			expectedErr:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := &linuxDetector{executor: tc.mockExecutor}
			status, err := detector.GetInstanceStatus(context.Background(), tc.instanceName)

			if tc.expectedErr {
				if err == nil {
					t.Fatal("Expected an error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Did not expect an error, but got: %v", err)
			}

			if status != tc.expectedStatus {
				t.Errorf("Expected status: %s, but got: %s", tc.expectedStatus, status)
			}
		})
	}
}

func TestWindowsDetector_GetInstanceStatus(t *testing.T) {
	testCases := []struct {
		name           string
		instanceName   string
		mockExecutor   *MockCommandExecutor
		expectedStatus string
		expectedErr    bool
	}{
		{
			name:         "Success - Active Service",
			instanceName: "dev",
			mockExecutor: &MockCommandExecutor{
				output: []byte("Running"),
				err:    nil,
			},
			expectedStatus: "active",
			expectedErr:    false,
		},
		{
			name:         "Success - Inactive Service",
			instanceName: "staging",
			mockExecutor: &MockCommandExecutor{
				output: []byte("Stopped"),
				err:    nil,
			},
			expectedStatus: "inactive",
			expectedErr:    false,
		},
		{
			name:         "Service Not Found",
			instanceName: "nonexistent",
			mockExecutor: &MockCommandExecutor{
				output: []byte("\r\n"),
				err:    nil,
			},
			expectedStatus: "not-found",
			expectedErr:    false,
		},
		{
			name:         "Command Execution Error",
			instanceName: "dev",
			mockExecutor: &MockCommandExecutor{
				output: nil,
				err:    errors.New("powershell command failed"),
			},
			expectedStatus: "",
			expectedErr:    true,
		},
		{
			name:         "Malformed Output",
			instanceName: "dev",
			mockExecutor: &MockCommandExecutor{
				output: []byte("some garbage text"),
				err:    nil,
			},
			expectedStatus: "unknown",
			expectedErr:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := &windowsDetector{executor: tc.mockExecutor}
			status, err := detector.GetInstanceStatus(context.Background(), tc.instanceName)

			if tc.expectedErr {
				if err == nil {
					t.Fatal("Expected an error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Did not expect an error, but got: %v", err)
			}

			if status != tc.expectedStatus {
				t.Errorf("Expected status: %s, but got: %s", tc.expectedStatus, status)
			}
		})
	}
}

// Integeration/E2E test.
// func TestIntegrationListInstances(t *testing.T) {
// 	instanceDetector, err := NewDetector()

// 	if err != nil {
// 		t.Error("Failed to get instanceDetector", err)
// 	}

// 	instances, err := instanceDetector.ListInstances(context.TODO())
// 	if err != nil {
// 		t.Error("List instances error: ", err)
// 	}
// 	fmt.Println(instances)
// }

// func TestIntegrationGetInstanceStatus(t *testing.T) {
// 	instancedetector, err := NewDetector()

// 	if err != nil {
// 		t.Error("Failed to get instance detector", err)
// 	}

// 	instanceStatus, err := instancedetector.GetInstanceStatus(context.TODO(), "test5")
// 	if err != nil {
// 		t.Error("List instances error: ", err)
// 	}
// 	fmt.Println(instanceStatus)
// }

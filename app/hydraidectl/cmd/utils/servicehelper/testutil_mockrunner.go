// servicehelper/testutil_mockrunner.go
package servicehelper

import (
	"fmt"
	"strings"
)

type MockRunner struct {
	Calls []string
	// Map: "cmd arg1 arg2" -> (output, err)
	Script map[string]mockResp
}

type mockResp struct {
	Out string
	Err error
}

func (m *MockRunner) Run(name string, args ...string) ([]byte, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	m.Calls = append(m.Calls, key)
	if r, ok := m.Script[key]; ok {
		return []byte(r.Out), r.Err
	}
	return nil, fmt.Errorf("unexpected command: %s", key)
}

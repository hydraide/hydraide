package instancehealth

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/joho/godotenv"
)

type InstanceHealth interface {
	GetHealthStatus(ctx context.Context, instance string) HealhStatus

	GetListHealthStatus(ctx context.Context, instances []string) []HealhStatus
}

type HealhStatus struct {
	Instance string
	Status   string
	Error    error
}

// Implementation of InstanceHealth interface
type instanceHealth struct {
	instanceController instancerunner.InstanceController
}

func (h *instanceHealth) GetHealthStatus(ctx context.Context, instance string) HealhStatus {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return h.performHealthCheck(ctx, instance)
}

func (h *instanceHealth) GetListHealthStatus(ctx context.Context, instances []string) []HealhStatus {
	totalCpus := runtime.NumCPU()
	numGoroutines := len(instances) / 3
	if numGoroutines < 2 {
		numGoroutines = 2
	} else if numGoroutines > totalCpus {
		numGoroutines = totalCpus
	}

	healthStatusResult := make([]HealhStatus, len(instances))

	// limit number of concurrent health checks to numGoroutines
	rateLimit := make(chan struct{}, numGoroutines)
	var wg sync.WaitGroup

	for i, instance := range instances {
		rateLimit <- struct{}{}
		wg.Add(1)

		go func(instance string, index int) {
			defer func() {
				<-rateLimit
			}()
			defer wg.Done()

			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			healthStatusResult[index] = h.performHealthCheck(ctx, instance)
		}(instance, i)
	}

	wg.Wait()
	return healthStatusResult
}

func (h *instanceHealth) performHealthCheck(ctx context.Context, instance string) HealhStatus {
	exists, err := h.instanceController.InstanceExists(ctx, instance)
	if err != nil {
		return HealhStatus{Instance: instance, Status: "unknown", Error: err}
	}
	if !exists {
		return HealhStatus{Instance: instance, Status: "unknown", Error: fmt.Errorf("instance does not exist")}
	}

	workDir, err := getWorkingDirectory(instance)
	if err != nil {
		return HealhStatus{Instance: instance, Status: "unknown", Error: err}
	}
	envPath := filepath.Join(workDir, ".env")
	envMap, err := godotenv.Read(envPath)
	if err != nil {
		return HealhStatus{Instance: instance, Status: "unknown", Error: err}
	}

	healthPortString, ok := envMap["HEALTH_CHECK_PORT"]
	if !ok {
		return HealhStatus{Instance: instance, Status: "unknown", Error: fmt.Errorf("HEALTH_CHECK_PORT is missing")}
	}

	healthport, err := strconv.Atoi(healthPortString)
	if err != nil {
		return HealhStatus{Instance: instance, Status: "unknown", Error: err}
	}

	url := fmt.Sprintf("http://localhost:%v/health", healthport)
	status, err := checkHealth(ctx, url)

	if err != nil {
		return HealhStatus{Instance: instance, Status: "unknown", Error: err}
	}
	return HealhStatus{Instance: instance, Status: status, Error: nil}
}

func checkHealth(ctx context.Context, url string) (string, error) {

	call, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	response, err := client.Do(call)
	if err != nil {
		return "", fmt.Errorf("health check request failed: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return "unhealthy", nil
	}

	return "healthy", nil
}

func NewInstanceHealth() InstanceHealth {
	instanceController := instancerunner.NewInstanceController()

	return &instanceHealth{instanceController: instanceController}
}

func getWorkingDirectory(instance string) (string, error) {
	serviceDir := filepath.Join("/etc", "systemd", "system")
	serviceFile := fmt.Sprintf("hydraserver-%s.service", instance)
	fullPath := filepath.Join(serviceDir, serviceFile)

	file, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments and empty lines
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Check for the "WorkingDirectory" key
		if strings.HasPrefix(line, "WorkingDirectory=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error while scanning file: %w", err)
	}

	return "", nil
}

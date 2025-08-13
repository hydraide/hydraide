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

// InstanceHealth defines the interface for checking the health status of a HydrAIDE instance.
// It provides methods for checking the health of a single instance and a list of instances.
type InstanceHealth interface {
	// GetHealthStatus performs a health check on a single instance.
	//
	// The check verifies the instance exists, reads its configuration to find the
	// health port, and makes an HTTP request to its health endpoint. A timeout
	// is applied via the context to prevent the check from hanging indefinitely.
	//
	// Parameters:
	//   ctx: The context for the operation, used for cancellation and deadlines.
	//   instance: The name of the HydrAIDE instance to check.
	//
	// Returns:
	//   A HealhStatus struct containing the instance name, status ("healthy",
	//   "unhealthy", or "unknown"), and an 'error' if one occurred.
	GetHealthStatus(ctx context.Context, instance string) HealhStatus

	// GetListHealthStatus performs health checks on a list of instances concurrently.
	//
	// It uses a bounded worker pool to limit the number of simultaneous goroutines,
	// preventing resource exhaustion. The degree of concurrency is dynamically
	// determined by the total number of instances and available CPU cores.
	//
	// Parameters:
	//   ctx: The context for the entire operation, used for cancellation and deadlines.
	//   instances: A slice of strings representing the names of the instances to check.
	//
	// Returns:
	//   A slice of HealhStatus structs, with each element corresponding to an
	//   instance from the input slice. The order of the output slice
	//   matches the order of the input.
	GetListHealthStatus(ctx context.Context, instances []string) []HealhStatus
}

// HealhStatus represents the outcome of a health check for a single instance.
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

// performHealthCheck carries out the full health check logic for a single instance.
// It performs multiple steps including checking existence, parsing configuration,
// and making an HTTP request to the health endpoint.
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

// checkHealth performs a low-level HTTP GET request to a URL.
// It returns "healthy" if a 200 OK status is received, "unhealthy" for any other
// status code, or an error if the request fails.
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

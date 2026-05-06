// Package portfinder finds free TCP port pairs for new HydrAIDE instances.
//
// HydrAIDE always exposes a gRPC port and a health-check port one above it
// (`gRPCPort + 1`). This package picks the lowest free pair starting from a
// configured base port, skipping ports that are already bound on the host or
// that are reserved by other HydrAIDE instances registered in buildmetadata.
package portfinder

import (
	"context"
	"fmt"
	"net"
	"strconv"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/env"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
)

const (
	// DefaultGRPCPort is the starting port for the gRPC server when no other
	// instance has claimed it.
	DefaultGRPCPort = 4900

	// PortBumpStep is added to the candidate gRPC port on each unsuccessful
	// attempt. Stepping by 10 keeps the (gRPC, health) pairs visually clean
	// across multiple instances on the same host (4900/4901, 4910/4911, …).
	PortBumpStep = 10

	// MaxAttempts caps the number of bumps before giving up. With the default
	// step of 10 this covers ports 4900–5890.
	MaxAttempts = 100
)

// IsPortFree reports whether the given TCP port can be bound on all
// interfaces. It opens and immediately closes a listener; a binding error
// (port in use, permission denied, etc.) is treated as not-free.
func IsPortFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// FindFreePair returns a (grpc, health) pair where health == grpc+1, starting
// at startPort and bumping by step on each attempt. A pair is accepted when
// neither port appears in `reserved` and both bind successfully on the host.
//
// Pass startPort=0 / step=0 / maxAttempts=0 to use the package defaults.
func FindFreePair(reserved map[int]bool, startPort, step, maxAttempts int) (int, int, error) {
	if startPort <= 0 {
		startPort = DefaultGRPCPort
	}
	if step <= 0 {
		step = PortBumpStep
	}
	if maxAttempts <= 0 {
		maxAttempts = MaxAttempts
	}

	for i := 0; i < maxAttempts; i++ {
		grpc := startPort + i*step
		health := grpc + 1
		if health > 65535 {
			break
		}
		if reserved[grpc] || reserved[health] {
			continue
		}
		if !IsPortFree(grpc) || !IsPortFree(health) {
			continue
		}
		return grpc, health, nil
	}

	return 0, 0, fmt.Errorf("no free port pair found in %d attempts starting at %d (step %d)",
		maxAttempts, startPort, step)
}

// ReservedPorts collects gRPC and health ports already claimed by other
// HydrAIDE instances on this host by reading each registered instance's
// `.env` file. Instances whose `.env` is missing or unreadable are silently
// skipped — the caller still benefits from the listening-check fallback in
// FindFreePair.
func ReservedPorts(ctx context.Context, fs filesystem.FileSystem) (map[int]bool, error) {
	bm, err := buildmeta.New(fs)
	if err != nil {
		return nil, fmt.Errorf("load buildmetadata: %w", err)
	}

	instances, err := bm.GetAllInstances()
	if err != nil {
		return nil, fmt.Errorf("enumerate instances: %w", err)
	}

	reserved := make(map[int]bool, len(instances)*2)
	for _, meta := range instances {
		e := env.New(fs, meta.BasePath)
		if !e.IsExists(ctx) {
			continue
		}
		s, err := e.Load(ctx)
		if err != nil {
			continue
		}
		if p, err := strconv.Atoi(s.HydrAIDEGRPCPort); err == nil && p > 0 {
			reserved[p] = true
		}
		if p, err := strconv.Atoi(s.HydrAIDEHealthCheckPort); err == nil && p > 0 {
			reserved[p] = true
		}
	}
	return reserved, nil
}

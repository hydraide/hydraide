package models

import (
	"log/slog"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CheckHydrAIDEServers performs a manual heartbeat check against all registered HydrAIDE servers.
//
// This function invokes the HydrAIDE SDK’s `Heartbeat()` method, which attempts to contact
// every known gRPC server instance. If one or more servers are unreachable at the time of the check,
// an aggregated error is returned, listing all failed connections.
//
// 🔍 When to use this:
// - For startup-time readiness checks
// - For human-facing status dashboards (e.g., "HydrAIDE OK ✅ / ERROR ❌")
// - For logging or alerting during long-running processes
//
// ⚠️ Important Notes:
//   - This is a *snapshot-style* check. It does **not** represent continuous availability.
//   - A failed heartbeat does **not** prevent the SDK from working in the future.
//     HydrAIDE SDKs include **automatic reconnection** logic that retries connections silently.
//
// ✅ Use this check for manual reassurance, status indicators, or admin tools —
// but don’t rely on it for mission-critical logic or blocking operations.
func CheckHydrAIDEServers(repo repo.Repo) error {
	// Create a timeout-aware context for safe cancellation
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Get the HydrAIDE SDK client from the shared repository
	h := repo.GetHydraidego()

	// Perform the heartbeat check
	if err := h.Heartbeat(ctx); err != nil {
		// Heartbeat failed — one or more servers were unreachable
		slog.Error("Error sending heartbeat", "error", err)
		return err
	}

	// All servers responded successfully — considered healthy
	return nil
}

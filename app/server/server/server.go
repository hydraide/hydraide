// Package server
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/core/zeus"
	"github.com/hydraide/hydraide/app/panichandler"
	"github.com/hydraide/hydraide/app/server/gateway"
	"github.com/hydraide/hydraide/app/server/observer"
	"github.com/hydraide/hydraide/app/server/telemetry"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	maxDepth        = 1
	foldersPerLevel = 1000
)

type Configuration struct {
	CertificateCrtFile string // Server CRT file path
	CertificateKeyFile string // Server Key file path
	ClientCAFile       string // Client CA file path
	// Hydra settings
	HydraServerPort       int   // the port where the hydra server listens
	HydraMaxMessageSize   int   // the maximum message size in bytes
	DefaultCloseAfterIdle int64 // the default close after idle time in seconds
	DefaultWriteInterval  int64 // the default write interval time in seconds
	DefaultFileSize       int64 // the default file size in bytes
	SystemResourceLogging bool  // if true, the system resource usage is logged
	// UseV2Engine enables the new append-only V2 storage engine.
	// V2 is significantly faster (32-112x) and uses less storage (50%).
	UseV2Engine bool
	// TelemetryEnabled enables real-time telemetry collection for the observe command.
	TelemetryEnabled bool
}

type Server interface {
	// Start starts the microservice
	Start() error
	// Stop stops the microservice gracefully
	Stop()
	// IsHydraRunning returns true if the hydra server is running
	IsHydraRunning() bool
}

type server struct {
	configuration      *Configuration
	observerCancelFunc context.CancelFunc
	mu                 sync.RWMutex
	serverRunning      bool
	grpcServer         *grpc.Server
	zeusInterface      zeus.Zeus
	observerInterface  observer.Observer
	telemetryCollector telemetry.Collector
	// shutdownCtx is cancelled at the very start of Stop(). Stream RPCs
	// (Subscribe*) listen on it so they can exit promptly and let
	// grpcServer.GracefulStop() return instead of blocking forever.
	shutdownCtx       context.Context
	shutdownCancel    context.CancelFunc
}

// gRPCGracefulStopTimeout caps how long we wait for in-flight RPCs to finish
// after Stop() is called. After this elapses we force-stop the gRPC server so
// the swamp-flush phase always gets a chance to run before the
// hydraidectl/systemd SIGKILL hits us at 180s.
const gRPCGracefulStopTimeout = 60 * time.Second

// shutdownWriteAllowlist enumerates RPC methods that are SAFE to serve while
// the engine is shutting down. Anything not on this list is rejected with
// codes.Unavailable as soon as MarkShuttingDown() flips the flag.
//
// Rule of thumb: read-only and observability methods stay open; anything that
// can mutate Swamp state (Set / Delete / Destroy / Lock / Increment / patch /
// Uint32Slice push|delete / RegisterSwamp / DeRegisterSwamp / CompactSwamp /
// Shift*) is rejected so the on-disk state can be flushed cleanly.
var shutdownWriteAllowlist = map[string]struct{}{
	"Heartbeat":              {},
	"Get":                    {},
	"GetAll":                 {},
	"GetByIndex":             {},
	"GetByKeys":              {},
	"GetByIndexStream":       {},
	"GetByIndexStreamFromMany": {},
	"GetStream":              {},
	"Count":                  {},
	"IsSwampExist":           {},
	"IsKeyExist":             {},
	"AreKeysExist":           {},
	"Uint32SliceSize":        {},
	"Uint32SliceIsValueExist": {},
	"GetTelemetryHistory":    {},
	"GetErrorDetails":        {},
	"GetTelemetryStats":      {},
	// Subscribe* are intentionally NOT here: shutdown context cancels them
	// directly inside the gateway so they exit with Unavailable.
}

func New(configuration *Configuration) Server {
	return &server{
		configuration: configuration,
	}
}

func (s *server) IsHydraRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serverRunning
}

func (s *server) Start() error {

	slog.Info("starting the HydrAIDE server...")
	// check if the server is already running
	s.mu.Lock()
	if s.serverRunning {
		s.mu.Unlock()
		return errors.New("HydrAIDE server is already running")
	}
	s.serverRunning = true
	s.mu.Unlock()

	settingsInterface := settings.New(maxDepth, foldersPerLevel)

	// Enable V2 engine if configured
	if s.configuration.UseV2Engine {
		if err := settingsInterface.SetEngine(settings.EngineV2); err != nil {
			slog.Warn("failed to set V2 engine, using V1", "error", err)
		} else {
			slog.Info("V2 append-only storage engine enabled")
		}
	}

	s.zeusInterface = zeus.New(settingsInterface, filesystem.New())
	s.zeusInterface.StartHydra()

	// shutdown context — cancelled in Stop(). Cheap to create here so the
	// gateway can hold a non-nil reference for the lifetime of the server.
	s.shutdownCtx, s.shutdownCancel = context.WithCancel(context.Background())

	var ctx context.Context
	ctx, s.observerCancelFunc = context.WithCancel(context.Background())
	s.observerInterface = observer.New(ctx, s.configuration.SystemResourceLogging)

	// Initialize telemetry collector if enabled
	if s.configuration.TelemetryEnabled {
		s.telemetryCollector = telemetry.New(telemetry.DefaultConfig())
		slog.Info("Telemetry collection enabled")
	}

	grpcServer := gateway.Gateway{
		ObserverInterface:     s.observerInterface,
		SettingsInterface:     settingsInterface,
		ZeusInterface:         s.zeusInterface,
		DefaultCloseAfterIdle: s.configuration.DefaultCloseAfterIdle,
		DefaultWriteInterval:  s.configuration.DefaultWriteInterval,
		DefaultFileSize:       s.configuration.DefaultFileSize,
		TelemetryCollector:    s.telemetryCollector,
		ShutdownCtx:           s.shutdownCtx,
	}

	// rejectIfShuttingDown returns a non-nil error when the engine is shutting
	// down AND the requested method is not on shutdownWriteAllowlist. The
	// returned error is codes.Unavailable so clients can retry on a healthy
	// instance. Reads keep flowing during the shutdown drain window.
	rejectIfShuttingDown := func(fullMethod string) error {
		hydraIface := s.zeusInterface.GetHydra()
		if hydraIface == nil || !hydraIface.IsShuttingDown() {
			return nil
		}
		method := extractMethodName(fullMethod)
		if _, ok := shutdownWriteAllowlist[method]; ok {
			return nil
		}
		return status.Error(codes.Unavailable, "server is shutting down — retry later")
	}

	unaryInterceptor := func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		startTime := time.Now()

		// Get the client's IP address
		clientIP := "unknown"
		if p, ok := peer.FromContext(ctx); ok {
			if addr, ok := p.Addr.(*net.TCPAddr); ok {
				clientIP = addr.IP.String()
			}
		}

		if rejectErr := rejectIfShuttingDown(info.FullMethod); rejectErr != nil {
			return nil, rejectErr
		}

		resp, err := handler(ctx, req)

		// Record telemetry if enabled
		if s.telemetryCollector != nil {
			duration := time.Since(startTime)
			event := telemetry.Event{
				Timestamp:  startTime,
				Method:     extractMethodName(info.FullMethod),
				SwampName:  extractSwampName(req),
				Keys:       extractKeys(req),
				DurationUs: duration.Microseconds(),
				Success:    err == nil,
				ClientIP:   clientIP,
			}

			if err != nil {
				if grpcErr, ok := status.FromError(err); ok {
					event.ErrorCode = grpcErr.Code().String()
					event.ErrorMsg = grpcErr.Message()
				} else {
					event.ErrorCode = "Unknown"
					event.ErrorMsg = err.Error()
				}
			}

			s.telemetryCollector.Record(event)
		}

		if err != nil {
			// Logging GRPC Server error
			if os.Getenv("GRPC_SERVER_ERROR_LOGGING") == "true" {
				if grpcErr, ok := status.FromError(err); ok {
					switch grpcErr.Code() {
					case codes.PermissionDenied:
						slog.Error("client request rejected: permission denied", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.Unauthenticated:
						slog.Error("client request rejected: unauthenticated", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.InvalidArgument:
						slog.Debug("client request rejected: invalid argument", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.ResourceExhausted:
						slog.Error("client request rejected: resource exhausted", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.FailedPrecondition:
						// slog removed, because this is often used for expected errors
					case codes.Aborted:
						slog.Debug("client request rejected: aborted", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.OutOfRange:
						slog.Debug("client request rejected: out of range", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.Unavailable:
						slog.Error("client request rejected: unavailable", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.DataLoss:
						slog.Error("client request rejected: data loss", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.Unknown:
						slog.Debug("client request rejected: unknown error", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.Internal:
						slog.Error("client request rejected: internal server error", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.Unimplemented:
						slog.Warn("client request rejected: unimplemented", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.DeadlineExceeded:
						slog.Debug("client request rejected: deadline exceeded", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					case codes.Canceled:
						slog.Debug("client request rejected: canceled by client", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					default:
						slog.Error("client request rejected: unknown grpc error code", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
					}
				} else {
					slog.Warn("client request rejected: non-gRPC error", "method", info.FullMethod, "clientIP", clientIP, "error", err.Error())
				}
			}
		}
		return resp, err
	}

	// start the main server and waiting for incoming requests
	panichandler.SafeGo("grpc-server", func() {

		// Resolve a TCP listener on the configured port. This is a hard failure: without a port, we cannot serve.
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.configuration.HydraServerPort))
		if err != nil {
			slog.Error("can not create listener for the hydra server", "error", err)
			panic("can not create listener for the HydrAIDE server")
		}

		// Load the server's certificate and private key from disk.
		// These identify the server to clients during the TLS handshake.
		srvCert, err := tls.LoadX509KeyPair(
			s.configuration.CertificateCrtFile,
			s.configuration.CertificateKeyFile,
		)
		if err != nil {
			slog.Error("failed to load server TLS keypair", "error", err)
			panic("failed to load server TLS keypair")
		}

		// Read the Client CA bundle. Clients must present certificates issued by this CA (mTLS).
		caPEM, err := os.ReadFile(s.configuration.ClientCAFile)
		if err != nil {
			slog.Error("failed to read client CA file", "error", err, "path", s.configuration.ClientCAFile)
			panic("failed to read client CA file")
		}

		// Build a certificate pool from the CA bundle to verify incoming client certs.
		clientCAPool := x509.NewCertPool()
		if !clientCAPool.AppendCertsFromPEM(caPEM) {
			slog.Error("failed to append client CA to pool", "path", s.configuration.ClientCAFile)
			panic("failed to append client CA to pool")
		}

		// Configure TLS:
		// - Present the server certificate
		// - REQUIRE and VERIFY client certificates (mutual TLS)
		// - Limit to TLS 1.3 for modern security defaults
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{srvCert},
			ClientAuth:   tls.RequireAndVerifyClientCert, // verify client certs
			ClientCAs:    clientCAPool,                   // client ca pool for mTLS
			MinVersion:   tls.VersionTLS13,
		}

		// Turn the TLS config into gRPC transport credentials.
		creds := credentials.NewTLS(tlsCfg)

		// Keepalive tuning to detect dead connections and free resources:
		// - Send a ping after 4m of idleness
		// - Close if no ACK within 20s
		// - Proactively close connections idle for 5m
		kaParams := keepalive.ServerParameters{
			// IF the connection is idle for 4 minutes, the server will send a keepalive ping.
			Time: 4 * time.Minute,
			// If there is no response to the keepalive ping within 20 seconds, the server will close the connection.
			Timeout: 20 * time.Second,
			// Maximum time a connection can be idle before it is closed.
			MaxConnectionIdle: 5 * time.Minute,
		}

		// enforcement policy to prevent clients from sending pings too frequently
		ep := keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second, // the minimum time a client should wait before sending a keepalive ping
			PermitWithoutStream: true,             // allow keepalive pings when there are no active streams
		}

		// Construct the gRPC server with:
		// - TLS creds (mTLS)
		// - Message size limits (protects memory / abuse)
		// - Unary interceptor for centralized logging/metrics/auth decisions
		// - Keepalive parameters (connection hygiene)
		streamInterceptor := func(
			srv interface{},
			ss grpc.ServerStream,
			info *grpc.StreamServerInfo,
			handler grpc.StreamHandler,
		) error {
			if rejectErr := rejectIfShuttingDown(info.FullMethod); rejectErr != nil {
				return rejectErr
			}
			return handler(srv, ss)
		}

		s.grpcServer = grpc.NewServer(
			grpc.Creds(creds),
			grpc.MaxSendMsgSize(s.configuration.HydraMaxMessageSize),
			grpc.MaxRecvMsgSize(s.configuration.HydraMaxMessageSize),
			grpc.UnaryInterceptor(unaryInterceptor),
			grpc.StreamInterceptor(streamInterceptor),
			grpc.KeepaliveParams(kaParams),
			grpc.KeepaliveEnforcementPolicy(ep),
		)

		// Register the Hydraide gRPC service implementation.
		hydrapb.RegisterHydraideServiceServer(s.grpcServer, &grpcServer)

		// Log the listening port for operational visibility.
		slog.Info(fmt.Sprintf("HydrAIDE server is listening on port: %d", s.configuration.HydraServerPort))

		// Start serving and block this goroutine until the server returns an error (e.g., shutdown).
		if err = s.grpcServer.Serve(lis); err != nil {
			slog.Error("can not start the HydrAIDE server", "error", err)
		}

	})

	return nil

}

// Stop stops the microservice gracefully.
//
// Order of operations matters — these phases must run in this exact sequence
// so that the on-disk state is always flushed before SIGKILL arrives at 180s
// from hydraidectl/systemd:
//
//  1. Mark Hydra as shutting down. The interceptors immediately start rejecting
//     new writes with codes.Unavailable; reads keep flowing.
//  2. Cancel shutdownCtx. Subscribe* stream handlers exit promptly.
//  3. grpcServer.GracefulStop() — wait up to gRPCGracefulStopTimeout for the
//     remaining in-flight unary RPCs to finish. After that fire-stop the gRPC
//     server so the socket is closed even if a handler is wedged.
//  4. WaitingForAllProcessesFinished — drain background goroutines.
//  5. zeus.StopHydra → hydra.GracefulStop — flush every open swamp to disk
//     (10s graceful + force-flush + 30s hard cap inside hydra.go).
//  6. Telemetry + observer cancellation.
func (s *server) Stop() {

	slog.Info("stopping the HydrAIDE server...")
	// check if the server is already stopped
	s.mu.Lock()
	if !s.serverRunning {
		s.mu.Unlock()
		slog.Info("HydrAIDE server stopped gracefully. Program is exiting...")
		return
	}
	s.serverRunning = false
	s.mu.Unlock()

	// Phase 1: flip shutdown flag so interceptors reject new writes.
	if s.zeusInterface != nil {
		if hydraIface := s.zeusInterface.GetHydra(); hydraIface != nil {
			hydraIface.MarkShuttingDown()
		}
	}

	// Phase 2: cancel the shutdown context so streams exit.
	if s.shutdownCancel != nil {
		s.shutdownCancel()
	}

	// Phase 3: gRPC graceful stop with hard timeout. We can't simply call
	// GracefulStop() inline — a wedged stream or an in-flight RPC that takes
	// longer than our 180s SIGKILL deadline would prevent the swamp flush
	// from ever running. Run it in a goroutine and force-stop after the cap.
	if s.grpcServer != nil {
		gracefulDone := make(chan struct{})
		panichandler.SafeGo("grpc-graceful-stop", func() {
			s.grpcServer.GracefulStop()
			close(gracefulDone)
		})

		select {
		case <-gracefulDone:
			slog.Info("gRPC server stopped gracefully")
		case <-time.After(gRPCGracefulStopTimeout):
			slog.Warn("gRPC GracefulStop did not return in time — forcing Stop()",
				"timeout", gRPCGracefulStopTimeout)
			s.grpcServer.Stop()
			<-gracefulDone
		}
	}

	// Phase 4: drain background goroutines.
	if s.observerInterface != nil {
		slog.Info("waiting for all processes to finish in the background")
		s.observerInterface.WaitingForAllProcessesFinished()
		slog.Info("all processes are finished in the background")
	}

	// Phase 5: flush every open swamp.
	if s.zeusInterface != nil {
		s.zeusInterface.StopHydra()
		slog.Info("HydrAIDE server stopped gracefully. Program is exiting...")
	}

	// Phase 6: telemetry + observer.
	if s.telemetryCollector != nil {
		s.telemetryCollector.Close()
	}

	if s.observerCancelFunc != nil {
		s.observerCancelFunc()
	}

}

// extractMethodName extracts the short method name from the full gRPC method path.
// e.g., "/hydraidepbgo.HydraideService/Get" -> "Get"
func extractMethodName(fullMethod string) string {
	for i := len(fullMethod) - 1; i >= 0; i-- {
		if fullMethod[i] == '/' {
			return fullMethod[i+1:]
		}
	}
	return fullMethod
}

// extractSwampName extracts the swamp name from various request types.
func extractSwampName(req interface{}) string {
	// Try common request types that have swamp information
	switch r := req.(type) {
	case *hydrapb.GetRequest:
		if len(r.GetSwamps()) > 0 {
			return formatSwampPath(r.GetSwamps()[0].GetIslandID(), r.GetSwamps()[0].GetSwampName())
		}
	case *hydrapb.SetRequest:
		if len(r.GetSwamps()) > 0 {
			return formatSwampPath(r.GetSwamps()[0].GetIslandID(), r.GetSwamps()[0].GetSwampName())
		}
	case *hydrapb.GetAllRequest:
		return formatSwampPath(r.GetIslandID(), r.GetSwampName())
	case *hydrapb.GetByIndexRequest:
		return formatSwampPath(r.GetIslandID(), r.GetSwampName())
	case *hydrapb.GetByKeysRequest:
		return formatSwampPath(r.GetIslandID(), r.GetSwampName())
	case *hydrapb.DeleteRequest:
		if len(r.GetSwamps()) > 0 {
			return formatSwampPath(r.GetSwamps()[0].GetIslandID(), r.GetSwamps()[0].GetSwampName())
		}
	case *hydrapb.RegisterSwampRequest:
		return r.GetSwampPattern()
	case *hydrapb.DeRegisterSwampRequest:
		return r.GetSwampPattern()
	}
	return ""
}

// formatSwampPath formats the island ID and swamp name into a full path.
func formatSwampPath(islandID uint64, swampName string) string {
	if islandID == 0 {
		return swampName
	}
	return fmt.Sprintf("%d/%s", islandID, swampName)
}

// extractKeys extracts the keys from various request types.
func extractKeys(req interface{}) []string {
	switch r := req.(type) {
	case *hydrapb.GetRequest:
		if len(r.GetSwamps()) > 0 {
			return r.GetSwamps()[0].GetKeys()
		}
	case *hydrapb.SetRequest:
		if len(r.GetSwamps()) > 0 {
			kv := r.GetSwamps()[0].GetKeyValues()
			keys := make([]string, 0, len(kv))
			for _, item := range kv {
				keys = append(keys, item.GetKey())
			}
			return keys
		}
	case *hydrapb.GetByKeysRequest:
		return r.GetKeys()
	case *hydrapb.DeleteRequest:
		if len(r.GetSwamps()) > 0 {
			return r.GetSwamps()[0].GetKeys()
		}
	}
	return nil
}

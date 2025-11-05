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
	s.zeusInterface = zeus.New(settingsInterface, filesystem.New())
	s.zeusInterface.StartHydra()

	var ctx context.Context
	ctx, s.observerCancelFunc = context.WithCancel(context.Background())
	s.observerInterface = observer.New(ctx, s.configuration.SystemResourceLogging)

	grpcServer := gateway.Gateway{
		ObserverInterface:     s.observerInterface,
		SettingsInterface:     settingsInterface,
		ZeusInterface:         s.zeusInterface,
		DefaultCloseAfterIdle: s.configuration.DefaultCloseAfterIdle,
		DefaultWriteInterval:  s.configuration.DefaultWriteInterval,
		DefaultFileSize:       s.configuration.DefaultFileSize,
	}

	unaryInterceptor := func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		// Get the client's IP address
		clientIP := "unknown"
		if p, ok := peer.FromContext(ctx); ok {
			if addr, ok := p.Addr.(*net.TCPAddr); ok {
				clientIP = addr.IP.String()
			}
		}

		resp, err := handler(ctx, req)
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
						slog.Debug("client request rejected: failed precondition", "method", info.FullMethod, "clientIP", clientIP, "error", grpcErr.Message())
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
		s.grpcServer = grpc.NewServer(
			grpc.Creds(creds),
			grpc.MaxSendMsgSize(s.configuration.HydraMaxMessageSize),
			grpc.MaxRecvMsgSize(s.configuration.HydraMaxMessageSize),
			grpc.UnaryInterceptor(unaryInterceptor), // add the interceptor
			grpc.KeepaliveParams(kaParams),          // keepalive parameters
			grpc.KeepaliveEnforcementPolicy(ep),     // enforcement policy
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

// Stop stops the microservice gracefully
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

	if s.grpcServer != nil {
		// stops the gRPC server gracefully because we don't want to get new requests from the crawler
		s.grpcServer.GracefulStop()
	}

	// waiting for all processes to finish. This is a blocker function until all processes are finished
	if s.observerInterface != nil {
		slog.Info("waiting for all processes to finish in the background")
		s.observerInterface.WaitingForAllProcessesFinished()
		slog.Info("all processes are finished in the background")
	}

	if s.zeusInterface != nil {
		// stop the HydrAIDE gracefully. This is a blocker function until all swamps are stopped gracefully
		s.zeusInterface.StopHydra()
		slog.Info("HydrAIDE server stopped gracefully. Program is exiting...")
	}

	// stop the observer's monitoring process
	s.observerCancelFunc()

}

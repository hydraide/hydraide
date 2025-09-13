package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/keepalive"
)

const (
	errorNoConnection = "there is no connection to the HydrAIDE server"
	errorConnection   = "error while connecting to the server"
)

type Client interface {
	Connect(connectionAnalysis bool) error
	CloseConnection()
	GetServiceClient(swampName name.Name) hydraidepbgo.HydraideServiceClient
	GetServiceClientAndHost(swampName name.Name) *ServiceClient
	GetUniqueServiceClients() []hydraidepbgo.HydraideServiceClient
	GetAllIslands() uint64
}

type ServiceClient struct {
	GrpcClient hydraidepbgo.HydraideServiceClient
	Host       string
}

type client struct {
	allIslands     uint64
	serviceClients map[uint64]*ServiceClient
	uniqueServices []hydraidepbgo.HydraideServiceClient
	connections    []*grpc.ClientConn
	maxMessageSize int
	servers        []*Server
	mu             sync.RWMutex
}

// Server represents a HydrAIDE server instance that is responsible for
// one or more Islands within the system.
//
// Each HydrAIDE server is assigned a non-overlapping range of Islands,
// which are deterministic hash slots derived from Swamp names. The client
// uses the IslandID to route requests to the correct server.
//
// Fields:
//   - Host:          The gRPC endpoint of the HydrAIDE server (e.g. "hydra01:4444")
//   - FromIsland:    The first Island (inclusive) that this server is responsible for
//   - ToIsland:      The last Island (inclusive) that this server handles
//   - CACrtPath:     Path to the server’s CA certificate (ca.crt), used to verify the server
//   - ClientCrtPath: Path to the client certificate (client.crt), issued by this server
//   - ClientKeyPath: Path to the client private key (client.key), used for mTLS authentication
//
// 🏝️ Why Islands?
// An Island is a physical-logical routing unit: a hash-based partition that
// stores one or more Swamps. Migrating an Island only requires copying its
// folder and updating the client’s routing table — no rehashing or server restart.
// This provides deterministic routing and simple horizontal scaling.
//
// 💡 Best practices:
// - Island ranges must not overlap between servers
// - Ranges should fully cover the configured `allIslands` space (e.g. 1–1000)
// - Each server has its own client certificate/key pair for secure mutual TLS authentication
type Server struct {
	Host          string
	FromIsland    uint64
	ToIsland      uint64
	CACrtPath     string // server’s CA certificate
	ClientCrtPath string // client certificate issued for this server
	ClientKeyPath string // client private key for mTLS
}

// New creates a new HydrAIDE client instance that connects to one or more servers,
// distributing Swamp requests based on deterministic Island-based routing.
//
// In HydrAIDE, every Swamp name maps to a specific Island (via hashing).
// The client computes the IslandID and resolves the target server by comparing
// it to the configured Island ranges.
//
// Parameters:
//   - servers: list of HydrAIDE servers with their Island ranges and TLS certificates
//   - allIslands: total number of Islands in the system (fixed, e.g. 1000)
//   - maxMessageSize: maximum allowed gRPC message size in bytes
//
// The returned Client instance provides:
//   - Stateless, deterministic Swamp → Island → server resolution
//   - Thread-safe management of gRPC connections
//   - Lazy connection establishment with `Connect()`
//   - Transparent horizontal scaling by Island ranges
//   - Per-server mTLS authentication: the client uses the
//     `ClientCrtPath` + `ClientKeyPath` pair for identity proof,
//     while verifying the server’s certificate via `CACrtPath`
//
// Example:
//
//	client := client.New([]*client.Server{
//	    {
//	        Host:          "hydra01:4444",
//	        FromIsland:    1,
//	        ToIsland:      500,
//	        CACrtPath:     "certs/hydra01/ca.crt",
//	        ClientCrtPath: "certs/hydra01/client.crt",
//	        ClientKeyPath: "certs/hydra01/client.key",
//	    },
//	    {
//	        Host:          "hydra02:4444",
//	        FromIsland:    501,
//	        ToIsland:      1000,
//	        CACrtPath:     "certs/hydra02/ca.crt",
//	        ClientCrtPath: "certs/hydra02/client.crt",
//	        ClientKeyPath: "certs/hydra02/client.key",
//	    },
//	}, 1000, 1024*1024*1024) // 1 GB max message size
//
//	err := client.Connect(true)
//	if err != nil {
//	    log.Fatal("connection failed:", err)
//	}
//
//	swamp := name.New().Sanctuary("users").Realm("profiles").Swamp("alex123")
//	service := client.GetServiceClient(swamp)
//	if service != nil {
//	    res, err := service.Read(...)
//	}
func New(servers []*Server, allIslands uint64, maxMessageSize int) Client {
	return &client{
		serviceClients: make(map[uint64]*ServiceClient),
		servers:        servers,
		allIslands:     allIslands,
		maxMessageSize: maxMessageSize,
	}
}

// Connect establishes gRPC connections to all configured HydrAIDE servers
// and maps each folder range to the corresponding service client.
//
// This function ensures that:
// - Each server is reachable and responsive via Heartbeat
// - TLS credentials are correctly loaded
// - gRPC retry policies and keepalive settings are applied
// - Every folder in the system has an associated gRPC client for routing
//
// Parameters:
//   - connectionAnalysis: if true, performs diagnostic ping for each Host
//     and logs detailed output (useful for dev/debug)
//
// Behavior:
//   - Iterates over all servers defined in the `client.servers` list
//   - For each server:
//   - Resolves TLS credentials from file
//   - Connects using `grpc.Dial()` with retry, backoff and keepalive
//   - Sends a heartbeat ping to validate server responsiveness
//   - Assigns the resulting gRPC client to each folder in that server’s range
//   - Populates the internal `serviceClients` and `connections` maps
//
// Errors:
//   - If a server fails TLS validation, connection, or heartbeat, the error is logged
//   - Connection proceeds for all other available servers — partial success is allowed
//
// Returns:
//   - nil if all servers connect successfully
//   - otherwise, returns an error and logs the connection failures
//
// Example:
//
//	err := client.Connect(true)
//	if err != nil {
//	    log.Fatal("HydrAIDE connection failed:", err)
//	}
func (c *client) Connect(connectionLog bool) error {

	c.mu.Lock()
	defer c.mu.Unlock()

	var errorMessages []error

	for _, server := range c.servers {

		func() {

			if connectionLog {
				pingHost(server.Host)
				grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(os.Stdout, os.Stderr, os.Stderr, 99))
			}

			serviceConfigJSON := `{
			  "methodConfig": [{
				"name": [{"service": "hydraidepbgo.HydraideService"}],
				"waitForReady": true,
				"retryPolicy": {
				  "MaxAttempts": 100,
				  "InitialBackoff": ".5s",
				  "MaxBackoff": "10s",
				  "BackoffMultiplier": 1.5,
				  "RetryableStatusCodes": ["UNAVAILABLE", "DEADLINE_EXCEEDED", "RESOURCE_EXHAUSTED", "INTERNAL", "UNKNOWN"]
				}
			  }]
			}`

			hostOnly := strings.Split(server.Host, ":")[0]

			// load the client key pair (mTLS)
			cliPair, certErr := tls.LoadX509KeyPair(server.ClientCrtPath, server.ClientKeyPath)
			if certErr != nil {
				slog.Error("failed to load client keypair", "error", certErr)
				errorMessages = append(errorMessages, certErr)
				return
			}

			// load CA cert from file (used for server verification)
			caPEM, readErr := os.ReadFile(server.CACrtPath)
			if readErr != nil {
				slog.Error("failed to read CA file", "error", readErr, "path", server.CACrtPath)
				errorMessages = append(errorMessages, readErr)
				return
			}
			roots := x509.NewCertPool()
			if !roots.AppendCertsFromPEM(caPEM) {
				err := fmt.Errorf("failed to append CA to pool: %s", server.CACrtPath)
				slog.Error(err.Error())
				errorMessages = append(errorMessages, err)
				return
			}

			// Create TLS config with client cert, CA cert, and SNI
			tlsCfg := &tls.Config{
				Certificates: []tls.Certificate{cliPair}, // client cert + key (mTLS)
				RootCAs:      roots,                      // check the server cert against the CA
				ServerName:   hostOnly,                   // SNI + hostname verification
				MinVersion:   tls.VersionTLS13,
			}

			creds := credentials.NewTLS(tlsCfg)

			opts := []grpc.DialOption{
				grpc.WithTransportCredentials(creds),
				grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(c.maxMessageSize)),
				grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(c.maxMessageSize)),
				grpc.WithDefaultServiceConfig(serviceConfigJSON),
				grpc.WithKeepaliveParams(keepalive.ClientParameters{
					Time:                60 * time.Second,
					Timeout:             10 * time.Second,
					PermitWithoutStream: true,
				}),
			}

			var conn *grpc.ClientConn
			var err error

			conn, err = grpc.NewClient(server.Host, opts...)
			if err != nil {

				slog.Error("error while connecting to the server: ", "error", err, "server", server.Host, "fromIsland", server.FromIsland, "toIsland", server.ToIsland)

				errorMessages = append(errorMessages, err)

				return

			}

			serviceClient := hydraidepbgo.NewHydraideServiceClient(conn)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pong, err := serviceClient.Heartbeat(ctx, &hydraidepbgo.HeartbeatRequest{Ping: "beat"})
			if err != nil || pong == nil || pong.Pong != "beat" {

				slog.Error("error while sending heartbeat request: ",
					"error", err,
					"server", server.Host,
					"fromIsland", server.FromIsland,
					"toIsland", server.ToIsland,
					"pongMessage", pong,
					"errorMessages", errorMessages)

				errorMessages = append(errorMessages, err)

				return
			}

			slog.Info("connected to the hydra server successfully")

			for island := server.FromIsland; island <= server.ToIsland; island++ {
				c.serviceClients[island] = &ServiceClient{
					GrpcClient: serviceClient,
					Host:       server.Host,
				}
			}

			c.connections = append(c.connections, conn)
			c.uniqueServices = append(c.uniqueServices, serviceClient)

		}()

	}

	if len(errorMessages) > 0 {
		return errors.New(errorConnection)
	}

	return nil

}

// CloseConnection gracefully shuts down all active gRPC connections
// previously established via Connect().
//
// This method ensures:
// - Each connection is closed safely
// - Any connection close errors are logged (but not returned)
// - Internal connection list is cleaned up
//
// Typically called when the application is shutting down, or when reconnecting
// with new configuration is required.
//
// Example:
//
//	defer client.CloseConnection()
func (c *client) CloseConnection() {

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, conn := range c.connections {
		if conn != nil {
			if err := conn.Close(); err != nil {
				slog.Error("error while closing connection", "error", err)
			}
		}
	}

}

// GetServiceClient returns the raw gRPC HydrAIDE service client for the given Swamp name.
//
// Internally:
// - Computes the folder number using the hash of the swamp name
// - Looks up the matching gRPC client from the internal serviceClients map
//
// Parameters:
//   - swampName: a fully-qualified HydrAIDE Name (Sanctuary → Realm → Swamp)
//
// Returns:
//   - hydraidepbgo.HydraideServiceClient (bound to the correct server)
//   - nil if no client is registered for the calculated folder
//
// Example:
//
//	name := name.New().Sanctuary("users").Realm("profiles").Swamp("alex123")
//	client := hydraClient.GetServiceClient(name)
//	if client != nil {
//	    res, _ := client.Read(...)
//	}
//
// Notes:
//   - The folder number is calculated from the swamp name hash, then routed to the correct server
//   - The lookup is thread-safe (uses a read lock)
//   - This method provides the low-level client only; use GetServiceClientWithMeta() if you need Host info or full routing metadata
func (c *client) GetServiceClient(swampName name.Name) hydraidepbgo.HydraideServiceClient {

	c.mu.RLock()
	defer c.mu.RUnlock()

	// get the folder number based on the swamp name
	folderNumber := swampName.GetIslandID(c.allIslands)

	// return with the service client based on the folder number
	if serviceClient, ok := c.serviceClients[folderNumber]; ok {
		return serviceClient.GrpcClient
	}

	slog.Error("error while getting service client by swamp name",
		"swampName", swampName.Get())

	return nil

}

// GetAllIslands returns the total number of Islands configured in the client.
func (c *client) GetAllIslands() uint64 {
	return c.allIslands
}

// GetServiceClientAndHost returns the full HydrAIDE service client wrapper for a given Swamp name.
//
// Unlike GetServiceClient(), which only returns the raw gRPC client,
// this method provides additional metadata, such as the Host identifier for the target server.
//
// Internally:
// - Computes the folder number by hashing the Swamp name
// - Resolves the target server from the folder-to-client map
//
// Returns:
// - *ServiceClient struct, which contains:
//   - `GrpcClient` → the actual gRPC HydrAIDEServiceClient
//   - `Host`       → the Host string of the resolved server (e.g. IP:port or logical name)
//
// - nil if no matching server is registered for the calculated folder
//
// Example:
//
//		swamp := name.New().Sanctuary("users").Realm("logs").Swamp("user123")
//		client := hydraClient.GetServiceClientAndHost(swamp)
//	 if client != nil {
//	   res, _ := client.GrpcClient.Read(...)
//	  fmt.Println("Resolved Host:", client.Host)
//	 }
//
// This is especially useful when:
// - Grouping Swamps by target server
// - Logging or debugging routing behavior
// - Performing multi-swamp operations where server affinity matters
//
// Note:
// - Thread-safe via internal read lock
// - The folder number is derived from the full Swamp name and `allIslands` total
func (c *client) GetServiceClientAndHost(swampName name.Name) *ServiceClient {

	c.mu.RLock()
	defer c.mu.RUnlock()

	// lekérdezzük a folder számát
	folderNumber := swampName.GetIslandID(c.allIslands)

	// a folder száma alapján visszaadjuk a klienst
	if serviceClient, ok := c.serviceClients[folderNumber]; ok {
		return serviceClient
	}

	slog.Error("error while getting service client by swamp name",
		"swampName", swampName.Get(),
		"error", errorNoConnection)

	return nil

}

// GetUniqueServiceClients returns all unique HydrAIDE service clients
// only for internal use.
func (c *client) GetUniqueServiceClients() []hydraidepbgo.HydraideServiceClient {

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.uniqueServices

}

// Function to check if a string is an IP address
func isIP(input string) bool {
	ip := net.ParseIP(input)
	return ip != nil
}

// Function to resolve hostname to IP address
func resolveHostname(hostname string) (string, error) {
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return "", err
	}
	return ips[0].String(), nil
}

// Function to ping an IP address
func ping(ip string) bool {
	out, err := exec.Command("ping", "-c", "4", ip).Output()
	if err != nil {
		return false
	}
	slog.Info("pinging the Host", "output", string(out))
	return true
}

// Main function to handle the input and process accordingly
func pingHost(hostnameOrIP string) {

	// remove the port number from the hostname
	hostnameOrIP = strings.Split(hostnameOrIP, ":")[0]

	if isIP(hostnameOrIP) {
		// If input is an IP address, just ping it
		if ping(hostnameOrIP) {
			slog.Info("the Host ping without error", "Host", hostnameOrIP)
		} else {
			slog.Warn("the Host does not ping", "Host", hostnameOrIP)
		}

	} else {
		ip, err := resolveHostname(hostnameOrIP)
		if err != nil {
			slog.Error("could not resolve hostname", "Host", hostnameOrIP, "error", err)
		}

		// If input is an IP address, just ping it
		if ping(ip) {
			slog.Info("the Host ping without error", "Host", ip)
		} else {
			slog.Warn("the Host does not ping", "Host", ip)
		}
	}
}

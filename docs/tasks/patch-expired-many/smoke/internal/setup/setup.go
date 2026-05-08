// Package setup wires up a HydrAIDE client + a fresh test swamp for the
// smoke binaries. Connection details come from env vars (see README.md).
package setup

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
)

// Connect builds a client + hydraidego instance from env vars, panics on
// connection failure.
func Connect() hydraidego.Hydraidego {
	host := envOr("HYDRAIDE_HOST", "localhost:4444")
	caPath := envOr("HYDRAIDE_CA_CRT", "./certs/ca.crt")
	crtPath := envOr("HYDRAIDE_CLIENT_CRT", "./certs/client.crt")
	keyPath := envOr("HYDRAIDE_CLIENT_KEY", "./certs/client.key")

	srv := []*client.Server{{
		Host:          host,
		FromIsland:    1,
		ToIsland:      1000,
		CACrtPath:     caPath,
		ClientCrtPath: crtPath,
		ClientKeyPath: keyPath,
	}}
	cl := client.New(srv, 1000, 64*1024*1024)
	if err := cl.Connect(false); err != nil {
		log.Fatalf("FAIL: connect %s: %v", host, err)
	}
	return hydraidego.New(cl)
}

// FreshSwamp registers a fresh msgpack-encoded swamp under the smoke
// realm with a unique suffix so concurrent runs don't collide. Returns
// the swamp name + a teardown closure to be deferred.
func FreshSwamp(h hydraidego.Hydraidego, suffix string) (name.Name, func()) {
	swamp := name.New().
		Sanctuary("smoke").
		Realm("patch-expired").
		Swamp(fmt.Sprintf("%s-%d", suffix, time.Now().UnixNano()))

	errs := h.RegisterSwamp(context.Background(), &hydraidego.RegisterSwampRequest{
		SwampPattern: swamp,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval:  time.Second,
			MaxFileSize:    8192,
			EncodingFormat: hydraidego.EncodingMsgPack,
		},
	})
	if len(errs) > 0 {
		log.Fatalf("FAIL: RegisterSwamp: %v", errs)
	}
	return swamp, func() {
		_ = h.Destroy(context.Background(), swamp)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

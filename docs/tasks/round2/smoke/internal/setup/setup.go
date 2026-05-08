// Package setup wires up a HydrAIDE client + fresh test swamps for the
// Round 2 smoke binaries. Connection details come from env vars (see
// README.md). The shape mirrors patch-expired-many/smoke/internal/setup.
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

// FreshSwamp registers a single fresh swamp under the round2 realm with
// a unique suffix.
func FreshSwamp(h hydraidego.Hydraidego, suffix string) (name.Name, func()) {
	swamp := name.New().
		Sanctuary("smoke").
		Realm("round2").
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

// FreshSwamps registers N fresh swamps for a multi-swamp test, with a
// shared run suffix so the names sit under the same wildcard pattern
// at the catalog level.
func FreshSwamps(h hydraidego.Hydraidego, suffix string, n int) ([]name.Name, func()) {
	swamps := make([]name.Name, 0, n)
	teardowns := make([]func(), 0, n)
	stamp := time.Now().UnixNano()
	for i := 0; i < n; i++ {
		sn := name.New().
			Sanctuary("smoke").
			Realm("round2").
			Swamp(fmt.Sprintf("%s-%d-%d", suffix, stamp, i))
		errs := h.RegisterSwamp(context.Background(), &hydraidego.RegisterSwampRequest{
			SwampPattern: sn,
			FilesystemSettings: &hydraidego.SwampFilesystemSettings{
				WriteInterval:  time.Second,
				MaxFileSize:    8192,
				EncodingFormat: hydraidego.EncodingMsgPack,
			},
		})
		if len(errs) > 0 {
			log.Fatalf("FAIL: RegisterSwamp %s: %v", sn.Get(), errs)
		}
		swamps = append(swamps, sn)
		teardowns = append(teardowns, func() {
			_ = h.Destroy(context.Background(), sn)
		})
	}
	return swamps, func() {
		for _, td := range teardowns {
			td()
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

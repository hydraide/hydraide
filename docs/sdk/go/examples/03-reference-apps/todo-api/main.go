package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/valyala/fasthttp"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	r, cleanup, err := setup.NewClient(ctx)
	cancel()
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer cleanup()

	regCtx, regCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := setup.Pattern(regCtx, r, SwampPattern()); err != nil {
		regCancel()
		log.Fatalf("register pattern: %v", err)
	}
	regCancel()

	srv := NewServer(r)

	fmt.Printf("todo-api ready on http://localhost%s\n", *addr)
	fmt.Println("import postman_collection.json (File → Import) for a ready-to-run workspace")
	fmt.Println("or try:  curl -s -X POST http://localhost" + *addr + "/todos -H 'content-type: application/json' -d '{\"title\":\"buy milk\"}'")

	server := &fasthttp.Server{
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		fmt.Println("shutting down")
		_ = server.Shutdown()
	}()

	if err := server.ListenAndServe(*addr); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

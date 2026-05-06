//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

func TestURLShortener(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, tracker := setup.NewTestClient(t)
	tracker.Track(LinksSwamp())
	tracker.Track(ClicksSwamp())

	if err := setup.Pattern(ctx, r, SwampPattern()); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	server := &fasthttp.Server{Handler: NewServer(r).Handler()}
	ln := fasthttputil.NewInmemoryListener()
	defer ln.Close()
	go func() { _ = server.Serve(ln) }()

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) { return ln.Dial() },
		},
		// Disable redirect following so we can assert on 302.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}

	// CREATE
	req, _ := http.NewRequest(http.MethodPost, "http://x/links",
		bytes.NewReader(must(json.Marshal(map[string]any{"url": "https://hydraide.io"}))))
	req.Header.Set("content-type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status: want 201, got %d", resp.StatusCode)
	}
	var created struct{ Code, URL string }
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if created.Code == "" || created.URL != "https://hydraide.io" {
		t.Fatalf("create response: %+v", created)
	}

	// REDIRECT three times → 302 each + click counter increments
	for i := 0; i < 3; i++ {
		resp, err := httpClient.Get("http://x/" + created.Code)
		if err != nil {
			t.Fatalf("redirect %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("redirect status: want 302, got %d", resp.StatusCode)
		}
		if got := resp.Header.Get("location"); got != "https://hydraide.io" {
			t.Fatalf("location header: %q", got)
		}
	}

	// STATS — clicks should be 3
	resp, err = httpClient.Get("http://x/links/" + created.Code + "/stats")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	var stats struct {
		Code, URL string
		Clicks    int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	resp.Body.Close()
	if stats.Clicks != 3 {
		t.Fatalf("clicks: want 3, got %d", stats.Clicks)
	}

	// DELETE
	delReq, _ := http.NewRequest(http.MethodDelete, "http://x/links/"+created.Code, nil)
	resp, err = httpClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: want 204, got %d", resp.StatusCode)
	}

	// REDIRECT after DELETE → 404
	resp, err = httpClient.Get("http://x/" + created.Code)
	if err != nil {
		t.Fatalf("redirect after delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("redirect after delete: want 404, got %d", resp.StatusCode)
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

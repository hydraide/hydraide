//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

func TestTodoAPI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, tracker := setup.NewTestClient(t)
	tracker.Track(Swamp())

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
		Timeout: 5 * time.Second,
	}

	// CREATE
	created := doJSON(t, httpClient, http.MethodPost, "http://x/todos",
		map[string]any{"title": "buy milk"}, http.StatusCreated)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("missing id in create response: %v", created)
	}
	if got := created["title"]; got != "buy milk" {
		t.Fatalf("title: want %q, got %v", "buy milk", got)
	}
	if got := created["done"]; got != false {
		t.Fatalf("done should default to false, got %v", got)
	}

	// READ
	read := doJSON(t, httpClient, http.MethodGet, "http://x/todos/"+id, nil, http.StatusOK)
	if read["id"] != id {
		t.Fatalf("read id mismatch")
	}

	// PATCH (flip done)
	patched := doJSON(t, httpClient, http.MethodPatch, "http://x/todos/"+id,
		map[string]any{"done": true}, http.StatusOK)
	if patched["done"] != true {
		t.Fatalf("done should be true after patch, got %v", patched["done"])
	}
	if patched["title"] != "buy milk" {
		t.Fatalf("title was changed by patch: %v", patched["title"])
	}

	// LIST (status=done should include this todo)
	listDone := doJSONArray(t, httpClient, http.MethodGet, "http://x/todos?status=done", http.StatusOK)
	if !containsID(listDone, id) {
		t.Fatalf("status=done list does not contain %s: %v", id, listDone)
	}
	listOpen := doJSONArray(t, httpClient, http.MethodGet, "http://x/todos?status=open", http.StatusOK)
	if containsID(listOpen, id) {
		t.Fatalf("status=open list should not contain done todo %s", id)
	}

	// DELETE
	resp, err := httpClient.Do(mustReq(t, http.MethodDelete, "http://x/todos/"+id, nil))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: want %d, got %d", http.StatusNoContent, resp.StatusCode)
	}

	// READ after DELETE → 404
	resp, err = httpClient.Get("http://x/todos/" + id)
	if err != nil {
		t.Fatalf("read after delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("read after delete: want 404, got %d", resp.StatusCode)
	}
}

// --- helpers ---------------------------------------------------------

func mustReq(t *testing.T, method, url string, body any) *http.Request {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("content-type", "application/json")
	return req
}

func doJSON(t *testing.T, c *http.Client, method, url string, body any, wantStatus int) map[string]any {
	t.Helper()
	resp, err := c.Do(mustReq(t, method, url, body))
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want status %d, got %d", method, url, wantStatus, resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil && !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func doJSONArray(t *testing.T, c *http.Client, method, url string, wantStatus int) []map[string]any {
	t.Helper()
	resp, err := c.Do(mustReq(t, method, url, nil))
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want %d, got %d", method, url, wantStatus, resp.StatusCode)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func containsID(items []map[string]any, id string) bool {
	for _, it := range items {
		if it["id"] == id {
			return true
		}
	}
	return false
}

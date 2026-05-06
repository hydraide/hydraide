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

func TestMultiTenant(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, tracker := setup.NewTestClient(t)
	tracker.Track(TenantSwamp("acme"))
	tracker.Track(TenantSwamp("zenith"))

	if err := setup.Pattern(ctx, r, SwampPattern()); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	server := &fasthttp.Server{Handler: NewServer(r).Handler()}
	ln := fasthttputil.NewInmemoryListener()
	defer ln.Close()
	go func() { _ = server.Serve(ln) }()

	c := &http.Client{
		Transport: &http.Transport{DialContext: func(_ context.Context, _, _ string) (net.Conn, error) { return ln.Dial() }},
		Timeout:   5 * time.Second,
	}

	// Two tenants, each gets one user. Their data must not bleed across.
	idAcme := createUser(t, c, "acme", "alice@acme.io", "Alice")
	idZenith := createUser(t, c, "zenith", "bob@zenith.co", "Bob")

	// Tenant isolation: list acme should only contain alice.
	listed := listUsers(t, c, "acme")
	if len(listed) != 1 || listed[0]["id"] != idAcme {
		t.Fatalf("acme list isolation broken: %v", listed)
	}
	listed = listUsers(t, c, "zenith")
	if len(listed) != 1 || listed[0]["id"] != idZenith {
		t.Fatalf("zenith list isolation broken: %v", listed)
	}

	// Patch alice's name.
	patched := doJSON(t, c, http.MethodPatch, "http://x/tenants/acme/users/"+idAcme,
		map[string]any{"name": "Alice Doe"}, http.StatusOK)
	if patched["name"] != "Alice Doe" {
		t.Fatalf("patch name failed: %v", patched)
	}
	if patched["email"] != "alice@acme.io" {
		t.Fatalf("patch corrupted email: %v", patched["email"])
	}

	// Claim alice — first should succeed, second concurrent claim must
	// fail because the lock is held.
	claim1 := doJSON(t, c, http.MethodPost, "http://x/tenants/acme/users/"+idAcme+"/claim",
		map[string]any{"holdSeconds": 5}, http.StatusOK)
	if claim1["acquired"] != true {
		t.Fatalf("first claim should succeed: %v", claim1)
	}
	resp := mustDo(t, c, http.MethodPost, "http://x/tenants/acme/users/"+idAcme+"/claim",
		map[string]any{"holdSeconds": 5})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second claim: want 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete tenant zenith → bob disappears.
	resp = mustDo(t, c, http.MethodDelete, "http://x/tenants/zenith", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete tenant: want 204, got %d", resp.StatusCode)
	}
	resp = mustDo(t, c, http.MethodGet, "http://x/tenants/zenith/users/"+idZenith, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("read after tenant delete: want 404, got %d", resp.StatusCode)
	}
}

// --- helpers ---------------------------------------------------------

func createUser(t *testing.T, c *http.Client, tenant, email, name string) string {
	t.Helper()
	resp := mustDo(t, c, http.MethodPost, "http://x/tenants/"+tenant+"/users",
		map[string]any{"email": email, "name": name})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user (%s): %d", tenant, resp.StatusCode)
	}
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	id, _ := v["id"].(string)
	if id == "" {
		t.Fatalf("missing id in create response")
	}
	return id
}

func listUsers(t *testing.T, c *http.Client, tenant string) []map[string]any {
	t.Helper()
	resp := mustDo(t, c, http.MethodGet, "http://x/tenants/"+tenant+"/users", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users (%s): %d", tenant, resp.StatusCode)
	}
	var v []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return v
}

func mustDo(t *testing.T, c *http.Client, method, url string, body any) *http.Response {
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
		t.Fatalf("build req: %v", err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func doJSON(t *testing.T, c *http.Client, method, url string, body any, wantStatus int) map[string]any {
	t.Helper()
	resp := mustDo(t, c, method, url, body)
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want %d, got %d", method, url, wantStatus, resp.StatusCode)
	}
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

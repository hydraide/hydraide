// multi-tenant-saas: per-tenant user store with business locks.
//
// Each tenant gets its own Catalog Swamp at
// apps/multi-tenant-saas/{tenantID}. Tenants are fully isolated:
// separate file on disk, separate lock domain, separate eviction
// timer, no row-level security to write or audit.
//
// Routes show the patterns most multi-tenant SaaS apps need:
//
//   - CRUD per user inside a tenant.
//   - Atomic field-level patches via CatalogPatchFields.
//   - "Claim" semantics via the Lock primitive (TTL-bounded business
//     lock so a crashed worker can never deadlock the system).
//   - Tenant deletion that removes the file from disk completely.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fasthttp/router"
	"github.com/google/uuid"
	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
	"github.com/valyala/fasthttp"
)

// User is the Catalog model.
type User struct {
	ID   string    `hydraide:"key"`
	Body *UserBody `hydraide:"value"`
}

type UserBody struct {
	Email     string    `msgpack:"email"`
	Name      string    `msgpack:"name"`
	IsActive  bool      `msgpack:"isActive"`
	CreatedAt time.Time `msgpack:"createdAt"`
}

// TenantSwamp is one Swamp per tenant.
func TenantSwamp(tenant string) name.Name {
	return name.New().Sanctuary("apps").Realm("multi-tenant-saas").Swamp(tenant)
}

// SwampPattern matches every tenant Swamp.
func SwampPattern() name.Name {
	return name.New().Sanctuary("apps").Realm("multi-tenant-saas").Swamp("*")
}

// Server bundles the HTTP handlers.
type Server struct {
	repo   repo.Repo
	router *router.Router
}

func NewServer(r repo.Repo) *Server {
	s := &Server{repo: r, router: router.New()}
	s.routes()
	return s
}

func (s *Server) Handler() fasthttp.RequestHandler { return s.router.Handler }

func (s *Server) routes() {
	s.router.POST("/tenants/{tenant}/users", s.createUser)
	s.router.GET("/tenants/{tenant}/users", s.listUsers)
	s.router.GET("/tenants/{tenant}/users/{id}", s.readUser)
	s.router.PATCH("/tenants/{tenant}/users/{id}", s.patchUser)
	s.router.DELETE("/tenants/{tenant}/users/{id}", s.deleteUser)
	s.router.POST("/tenants/{tenant}/users/{id}/claim", s.claimUser)
	s.router.DELETE("/tenants/{tenant}", s.deleteTenant)
}

// --- request / response shapes ---------------------------------------

type createUserReq struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type patchUserReq struct {
	Email    *string `json:"email,omitempty"`
	Name     *string `json:"name,omitempty"`
	IsActive *bool   `json:"isActive,omitempty"`
}

type userView struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

type claimReq struct {
	HoldSeconds int `json:"holdSeconds"`
}

type claimResp struct {
	Acquired  bool   `json:"acquired"`
	HeldUntil string `json:"heldUntil,omitempty"`
}

func toView(u *User) userView {
	v := userView{ID: u.ID}
	if u.Body != nil {
		v.Email = u.Body.Email
		v.Name = u.Body.Name
		v.IsActive = u.Body.IsActive
		v.CreatedAt = u.Body.CreatedAt
	}
	return v
}

// --- handlers --------------------------------------------------------

func (s *Server) createUser(ctx *fasthttp.RequestCtx) {
	tenant, _ := ctx.UserValue("tenant").(string)
	if !validTenant(tenant) {
		writeErr(ctx, fasthttp.StatusBadRequest, "invalid tenant")
		return
	}
	var req createUserReq
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeErr(ctx, fasthttp.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		writeErr(ctx, fasthttp.StatusBadRequest, "email is required")
		return
	}
	user := &User{
		ID: uuid.New().String(),
		Body: &UserBody{
			Email:     req.Email,
			Name:      req.Name,
			IsActive:  true,
			CreatedAt: time.Now().UTC(),
		},
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.repo.GetHydraidego().CatalogSave(dbCtx, TenantSwamp(tenant), user); err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "save: "+err.Error())
		return
	}
	writeJSON(ctx, fasthttp.StatusCreated, toView(user))
}

func (s *Server) listUsers(ctx *fasthttp.RequestCtx) {
	tenant, _ := ctx.UserValue("tenant").(string)
	if !validTenant(tenant) {
		writeErr(ctx, fasthttp.StatusBadRequest, "invalid tenant")
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := make([]userView, 0, 16)
	err := s.repo.GetHydraidego().CatalogReadMany(dbCtx, TenantSwamp(tenant), &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}, User{}, func(model any) error {
		out = append(out, toView(model.(*User)))
		return nil
	})
	if err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "list: "+err.Error())
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, out)
}

func (s *Server) readUser(ctx *fasthttp.RequestCtx) {
	tenant, _ := ctx.UserValue("tenant").(string)
	id, _ := ctx.UserValue("id").(string)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	got := &User{}
	if err := s.repo.GetHydraidego().CatalogRead(dbCtx, TenantSwamp(tenant), id, got); err != nil {
		writeErr(ctx, fasthttp.StatusNotFound, "not found")
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, toView(got))
}

func (s *Server) patchUser(ctx *fasthttp.RequestCtx) {
	tenant, _ := ctx.UserValue("tenant").(string)
	id, _ := ctx.UserValue("id").(string)
	var req patchUserReq
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeErr(ctx, fasthttp.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	fields := map[string]any{}
	if req.Email != nil {
		fields["email"] = *req.Email
	}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.IsActive != nil {
		fields["isActive"] = *req.IsActive
	}
	if len(fields) == 0 {
		writeErr(ctx, fasthttp.StatusBadRequest, "no patchable fields supplied")
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := s.repo.GetHydraidego().CatalogPatchFields(dbCtx, TenantSwamp(tenant), id, fields)
	if err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "patch: "+err.Error())
		return
	}
	if status == hydraidego.PatchStatusKeyNotFound {
		writeErr(ctx, fasthttp.StatusNotFound, "not found")
		return
	}
	got := &User{}
	if err := s.repo.GetHydraidego().CatalogRead(dbCtx, TenantSwamp(tenant), id, got); err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "reload: "+err.Error())
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, toView(got))
}

func (s *Server) deleteUser(ctx *fasthttp.RequestCtx) {
	tenant, _ := ctx.UserValue("tenant").(string)
	id, _ := ctx.UserValue("id").(string)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.repo.GetHydraidego().CatalogDelete(dbCtx, TenantSwamp(tenant), id); err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "delete: "+err.Error())
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

// claimUser acquires a TTL-bounded business lock keyed by tenant+user.
// Two concurrent claim requests on the same user — even from different
// processes — will not both succeed. The TTL guarantees the lock cannot
// deadlock if the holder crashes.
func (s *Server) claimUser(ctx *fasthttp.RequestCtx) {
	tenant, _ := ctx.UserValue("tenant").(string)
	id, _ := ctx.UserValue("id").(string)
	var req claimReq
	if len(ctx.PostBody()) > 0 {
		_ = json.Unmarshal(ctx.PostBody(), &req)
	}
	if req.HoldSeconds <= 0 {
		req.HoldSeconds = 30
	}
	lockName := fmt.Sprintf("multi-tenant-saas:%s:%s", tenant, id)
	ttl := time.Duration(req.HoldSeconds) * time.Second

	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := s.repo.GetHydraidego().Lock(dbCtx, lockName, ttl); err != nil {
		writeErr(ctx, fasthttp.StatusConflict, "lock contended: "+err.Error())
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, claimResp{
		Acquired:  true,
		HeldUntil: time.Now().UTC().Add(ttl).Format(time.RFC3339),
	})
}

// deleteTenant removes the entire tenant Swamp from disk. Compare with a
// shared-table multi-tenant model where you would have to issue a DELETE
// across millions of rows.
func (s *Server) deleteTenant(ctx *fasthttp.RequestCtx) {
	tenant, _ := ctx.UserValue("tenant").(string)
	if !validTenant(tenant) {
		writeErr(ctx, fasthttp.StatusBadRequest, "invalid tenant")
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.repo.GetHydraidego().Destroy(dbCtx, TenantSwamp(tenant)); err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "destroy: "+err.Error())
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

// --- helpers ---------------------------------------------------------

func validTenant(t string) bool {
	if t == "" || len(t) > 64 {
		return false
	}
	for _, r := range t {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

func writeJSON(ctx *fasthttp.RequestCtx, status int, v any) {
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/json; charset=utf-8")
	body, _ := json.Marshal(v)
	_, _ = ctx.Write(body)
}

func writeErr(ctx *fasthttp.RequestCtx, status int, msg string) {
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/json; charset=utf-8")
	_, _ = fmt.Fprintf(ctx, `{"error":%q}`, msg)
}

// --- main ------------------------------------------------------------

func main() {
	addr := flag.String("addr", ":8082", "listen address")
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

	fmt.Printf("multi-tenant-saas ready on http://localhost%s\n", *addr)
	fmt.Println("import postman_collection.json (File → Import) for a ready-to-run workspace")
	fmt.Println(`or try:  curl -s -X POST http://localhost` + *addr + `/tenants/acme/users -H 'content-type: application/json' -d '{"email":"alice@acme.io","name":"Alice"}'`)

	server := &fasthttp.Server{
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		_ = server.Shutdown()
	}()
	if err := server.ListenAndServe(*addr); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

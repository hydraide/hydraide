// url-shortener: short-code → long-URL service backed by HydrAIDE.
//
// Two swamps:
//
//   - apps/url-shortener/links   — Catalog of Link records (code → URL).
//   - apps/url-shortener/clicks  — int64 click counters keyed by code,
//                                  mutated only by IncrementInt64.
//
// Why two swamps? The redirect path is read-heavy and the counter is
// write-heavy. Splitting them lets each swamp evict on its own schedule
// and gives the counter swamp a tighter write interval if needed.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fasthttp/router"
	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
	"github.com/valyala/fasthttp"
)

// Link is the Catalog model.
type Link struct {
	Code string    `hydraide:"key"`
	Body *LinkBody `hydraide:"value"`
}

type LinkBody struct {
	URL       string    `msgpack:"url"`
	CreatedAt time.Time `msgpack:"createdAt"`
}

// LinksSwamp stores code → URL.
func LinksSwamp() name.Name {
	return name.New().Sanctuary("apps").Realm("url-shortener").Swamp("links")
}

// ClicksSwamp stores per-code int64 counters.
func ClicksSwamp() name.Name {
	return name.New().Sanctuary("apps").Realm("url-shortener").Swamp("clicks")
}

// SwampPattern matches both swamps under apps/url-shortener/*.
func SwampPattern() name.Name {
	return name.New().Sanctuary("apps").Realm("url-shortener").Swamp("*")
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
	s.router.POST("/links", s.create)
	s.router.GET("/links/{code}/stats", s.stats)
	s.router.DELETE("/links/{code}", s.delete)
	s.router.GET("/{code}", s.redirect)
}

type createReq struct {
	URL string `json:"url"`
}

type linkView struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

type statsView struct {
	Code   string `json:"code"`
	URL    string `json:"url"`
	Clicks int64  `json:"clicks"`
}

func (s *Server) create(ctx *fasthttp.RequestCtx) {
	var req createReq
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeErr(ctx, fasthttp.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if _, err := url.ParseRequestURI(req.URL); err != nil {
		writeErr(ctx, fasthttp.StatusBadRequest, "url must be absolute and parseable")
		return
	}
	code, err := newCode()
	if err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "code: "+err.Error())
		return
	}
	link := &Link{
		Code: code,
		Body: &LinkBody{URL: req.URL, CreatedAt: time.Now().UTC()},
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.repo.GetHydraidego().CatalogSave(dbCtx, LinksSwamp(), link); err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "save: "+err.Error())
		return
	}
	writeJSON(ctx, fasthttp.StatusCreated, linkView{Code: code, URL: req.URL, CreatedAt: link.Body.CreatedAt})
}

func (s *Server) redirect(ctx *fasthttp.RequestCtx) {
	code, _ := ctx.UserValue("code").(string)
	if code == "" {
		writeErr(ctx, fasthttp.StatusNotFound, "code missing")
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	got := &Link{}
	if err := s.repo.GetHydraidego().CatalogRead(dbCtx, LinksSwamp(), code, got); err != nil {
		writeErr(ctx, fasthttp.StatusNotFound, "no such code")
		return
	}

	// Atomic click counter — never races, no read-modify-write.
	if _, _, err := s.repo.GetHydraidego().IncrementInt64(dbCtx, ClicksSwamp(), code, 1, nil, nil, nil); err != nil {
		log.Printf("counter %s: %v", code, err)
	}

	ctx.Response.Header.Set("location", got.Body.URL)
	ctx.SetStatusCode(fasthttp.StatusFound)
}

func (s *Server) stats(ctx *fasthttp.RequestCtx) {
	code, _ := ctx.UserValue("code").(string)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	got := &Link{}
	if err := s.repo.GetHydraidego().CatalogRead(dbCtx, LinksSwamp(), code, got); err != nil {
		writeErr(ctx, fasthttp.StatusNotFound, "no such code")
		return
	}

	clicks := readClickCount(dbCtx, s.repo, code)

	writeJSON(ctx, fasthttp.StatusOK, statsView{Code: code, URL: got.Body.URL, Clicks: clicks})
}

func (s *Server) delete(ctx *fasthttp.RequestCtx) {
	code, _ := ctx.UserValue("code").(string)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = s.repo.GetHydraidego().CatalogDelete(dbCtx, LinksSwamp(), code)
	_ = s.repo.GetHydraidego().CatalogDelete(dbCtx, ClicksSwamp(), code)
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

// readClickCount fetches the current int64 counter via a typed Catalog
// read using a tiny internal model that exposes the value field.
func readClickCount(ctx context.Context, r repo.Repo, code string) int64 {
	type ClickCounter struct {
		Code  string `hydraide:"key"`
		Value int64  `hydraide:"value"`
	}
	got := &ClickCounter{}
	if err := r.GetHydraidego().CatalogRead(ctx, ClicksSwamp(), code, got); err != nil {
		return 0
	}
	return got.Value
}

// --- helpers ---------------------------------------------------------

func newCode() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	s := base64.RawURLEncoding.EncodeToString(buf)
	return strings.TrimRight(s, "="), nil
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
	addr := flag.String("addr", ":8081", "listen address")
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

	fmt.Printf("url-shortener ready on http://localhost%s\n", *addr)
	fmt.Println("import postman_collection.json (File → Import) for a ready-to-run workspace")
	fmt.Println(`or try:  curl -s -X POST http://localhost` + *addr + `/links -H 'content-type: application/json' -d '{"url":"https://hydraide.io"}'`)

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

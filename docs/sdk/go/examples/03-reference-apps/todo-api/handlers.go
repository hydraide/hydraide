package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fasthttp/router"
	"github.com/google/uuid"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
	"github.com/valyala/fasthttp"
)

// Server wires the HydrAIDE repo into HTTP handlers and exposes a
// fasthttp router. The struct is the single integration surface — tests
// build it directly and call the router via in-memory fasthttp without
// opening a port.
type Server struct {
	repo   repo.Repo
	router *router.Router
}

// NewServer constructs a Server with the routes registered. Pattern
// registration must happen before NewServer is called (see main.go).
func NewServer(r repo.Repo) *Server {
	s := &Server{repo: r, router: router.New()}
	s.routes()
	return s
}

// Handler returns the fasthttp.RequestHandler the user mounts on a
// listener.
func (s *Server) Handler() fasthttp.RequestHandler { return s.router.Handler }

func (s *Server) routes() {
	s.router.POST("/todos", s.createTodo)
	s.router.GET("/todos", s.listTodos)
	s.router.GET("/todos/{id}", s.readTodo)
	s.router.PATCH("/todos/{id}", s.patchTodo)
	s.router.DELETE("/todos/{id}", s.deleteTodo)
}

// --- request / response shapes ---------------------------------------

type createReq struct {
	Title string    `json:"title"`
	DueAt time.Time `json:"dueAt"`
}

type todoView struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	DueAt     time.Time `json:"dueAt"`
	CreatedAt time.Time `json:"createdAt"`
}

type patchReq struct {
	Title *string    `json:"title,omitempty"`
	Done  *bool      `json:"done,omitempty"`
	DueAt *time.Time `json:"dueAt,omitempty"`
}

func toView(t *Todo) todoView {
	v := todoView{ID: t.ID}
	if t.Body != nil {
		v.Title = t.Body.Title
		v.Done = t.Body.Done
		v.DueAt = t.Body.DueAt
		v.CreatedAt = t.Body.CreatedAt
	}
	return v
}

// --- handlers --------------------------------------------------------

func (s *Server) createTodo(ctx *fasthttp.RequestCtx) {
	var req createReq
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeErr(ctx, fasthttp.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeErr(ctx, fasthttp.StatusBadRequest, "title is required")
		return
	}

	todo := &Todo{
		ID: uuid.New().String(),
		Body: &TodoBody{
			Title:     req.Title,
			Done:      false,
			DueAt:     req.DueAt,
			CreatedAt: time.Now().UTC(),
		},
	}

	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.repo.GetHydraidego().CatalogSave(dbCtx, Swamp(), todo); err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "save: "+err.Error())
		return
	}
	writeJSON(ctx, fasthttp.StatusCreated, toView(todo))
}

func (s *Server) listTodos(ctx *fasthttp.RequestCtx) {
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	var filter *hydraidego.FilterGroup
	switch string(ctx.QueryArgs().Peek("status")) {
	case "open":
		filter = hydraidego.FilterAND(
			hydraidego.FilterBytesFieldBool(hydraidego.Equal, "done", false),
		)
	case "done":
		filter = hydraidego.FilterAND(
			hydraidego.FilterBytesFieldBool(hydraidego.Equal, "done", true),
		)
	}

	out := make([]todoView, 0, 16)
	err := s.repo.GetHydraidego().CatalogReadManyStream(dbCtx, Swamp(), index, filter, Todo{}, func(model any) error {
		t := model.(*Todo)
		out = append(out, toView(t))
		return nil
	})
	if err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "list: "+err.Error())
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, out)
}

func (s *Server) readTodo(ctx *fasthttp.RequestCtx) {
	id, _ := ctx.UserValue("id").(string)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	got := &Todo{}
	err := s.repo.GetHydraidego().CatalogRead(dbCtx, Swamp(), id, got)
	if err != nil {
		writeErr(ctx, fasthttp.StatusNotFound, "not found")
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, toView(got))
}

func (s *Server) patchTodo(ctx *fasthttp.RequestCtx) {
	id, _ := ctx.UserValue("id").(string)

	var req patchReq
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeErr(ctx, fasthttp.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	fields := map[string]any{}
	if req.Title != nil {
		fields["title"] = *req.Title
	}
	if req.Done != nil {
		fields["done"] = *req.Done
	}
	if req.DueAt != nil {
		fields["dueAt"] = *req.DueAt
	}
	if len(fields) == 0 {
		writeErr(ctx, fasthttp.StatusBadRequest, "no patchable fields supplied")
		return
	}

	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := s.repo.GetHydraidego().CatalogPatchFields(dbCtx, Swamp(), id, fields)
	if err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "patch: "+err.Error())
		return
	}
	if status == hydraidego.PatchStatusKeyNotFound {
		writeErr(ctx, fasthttp.StatusNotFound, "not found")
		return
	}

	got := &Todo{}
	if err := s.repo.GetHydraidego().CatalogRead(dbCtx, Swamp(), id, got); err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "reload: "+err.Error())
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, toView(got))
}

func (s *Server) deleteTodo(ctx *fasthttp.RequestCtx) {
	id, _ := ctx.UserValue("id").(string)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.repo.GetHydraidego().CatalogDelete(dbCtx, Swamp(), id); err != nil {
		writeErr(ctx, fasthttp.StatusInternalServerError, "delete: "+err.Error())
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

// --- response helpers ------------------------------------------------

func writeJSON(ctx *fasthttp.RequestCtx, status int, v any) {
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/json; charset=utf-8")
	body, err := json.Marshal(v)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		_, _ = fmt.Fprintf(ctx, `{"error":"encode: %s"}`, err)
		return
	}
	_, _ = ctx.Write(body)
}

func writeErr(ctx *fasthttp.RequestCtx, status int, msg string) {
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/json; charset=utf-8")
	_, _ = fmt.Fprintf(ctx, `{"error":%q}`, msg)
}

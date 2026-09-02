package hello

import (
	"context"
	"net/http"
	"strings"

	"github.com/guilhermelinosp/golang-api-template/internal/api"
)

// Handler exposes the greeting endpoints. It implements the transport-neutral
// api.Handler contract; registering it happens through plain Route values:
//
//	for _, r := range handler.Routes() { ... }
type Handler struct {
	service Service
}

// NewHandler wires the handler to its service.
func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// Routes declares this module's contribution to /api/v1.
// Three flavors on purpose — they document every input style a typical
// endpoint needs: query string, path wildcard and JSON body.
func (h *Handler) Routes() []api.Route {
	return []api.Route{
		{Method: api.MethodGet, Path: "/hello", Handler: api.HandlerFunc(h.greetByQuery)},
		{Method: api.MethodGet, Path: "/hello/{name}", Handler: api.HandlerFunc(h.greetByPath)},
		{Method: api.MethodPost, Path: "/hello", Handler: api.HandlerFunc(h.greetByBody)},
	}
}

// greetResponse is the wire shape of a successful greeting.
type greetResponse struct {
	Message string `json:"message"`
}

func (h *Handler) greetByQuery(ctx context.Context, req api.Request) (api.Response, error) {
	name := normalize(req.Query("name"))
	msg, err := h.service.Greet(ctx, name)
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusOK, greetResponse{Message: msg}), nil
}

func (h *Handler) greetByPath(ctx context.Context, req api.Request) (api.Response, error) {
	name := normalize(req.Param("name"))
	if name == "" {
		return api.Response{}, api.Validation("name", "path parameter is required")
	}
	msg, err := h.service.Greet(ctx, name)
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusOK, greetResponse{Message: msg}), nil
}

// greetRequest is the strict body contract for POST /hello. Unknown fields
// are rejected by the abstraction (typo-proof API by default).
type greetRequest struct {
	Name string `json:"name"`
}

func (h *Handler) greetByBody(ctx context.Context, req api.Request) (api.Response, error) {
	var in greetRequest
	if err := req.Bind(&in); err != nil {
		return api.Response{}, err // already a 400/413-class application error
	}
	name := normalize(in.Name)
	if name == "" {
		return api.Response{}, api.Validation("name", "is required")
	}
	msg, err := h.service.Greet(ctx, name)
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusCreated, greetResponse{Message: msg}), nil
}

func normalize(name string) string { return strings.TrimSpace(name) }

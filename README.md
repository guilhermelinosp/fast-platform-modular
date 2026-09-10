# golang-api-template

> Opinionated, production-ready GitHub template for Go HTTP APIs.
> Boot with **~70–80% of infrastructure done** — you write business logic.

[![pipeline](https://github.com/guilhermelinosp/fast-platform-modular/actions/workflows/pipeline.yml/badge.svg)](https://github.com/guilhermelinosp/fast-platform-modular/actions/workflows/pipeline.yml)
[![pr-check](https://github.com/guilhermelinosp/fast-platform-modular/actions/workflows/pr-check.yml/badge.svg)](https://github.com/guilhermelinosp/fast-platform-modular/actions/workflows/pr-check.yml)
[![CodeQL](https://github.com/guilhermelinosp/fast-platform-modular/actions/workflows/codeql.yml/badge.svg)](https://github.com/guilhermelinosp/fast-platform-modular/actions/workflows/codeql.yml)

Three non-negotiable statements about this codebase:

```text
Gin is an implementation detail.
hellnet-lib-telemetry is the standard observability layer.
Business logic does not depend on Gin.
```

---

## What you get for free

| Capability | Where it comes from |
|---|---|---|
| HTTP routing, handlers, validation, error envelope | [hellnet-lib-api](https://github.com/guilhermelinosp/hellnet-lib-api) (`api` + `adapter`) |
| Runtime configuration (`HELLNET_*` envs) | hellnet-lib-api (`config`) |
| Reusable `http.Server` lifecycle | hellnet-lib-api (`server`) |
| Structured logging (`slog`, JSON, trace-correlated) | [hellnet-lib-telemetry](https://github.com/guilhermelinosp/hellnet-lib-telemetry) |
| Metrics (`http_requests_total`, `http_request_duration_seconds`, inflight, sizes, errors, runtime) | hellnet-lib-telemetry |
| Distributed tracing (OTLP via otelhttp) | hellnet-lib-telemetry |
| `/live` `/ready` `/health` `/metrics` endpoints | hellnet-lib-telemetry + hellnet-lib-api platform routes |
| Sensitive-data redaction in logs | hellnet-lib-telemetry (`RedactSensitive`) |
| Graceful shutdown with correct telemetry flush order | hellnet-lib-api `server` + lib `Shutdown()` |
| Secure timeouts, request-id, security headers, CORS | hellnet-lib-api adapter |
| Tests via stdlib only (`testing` + `httptest`) | template suites |
| CI: test/lint/CodeQL/dependency-review/govulncheck | `.github/workflows` |
| Release: semver tag → GH release → goreleaser binaries → image | org reusable workflows + GoReleaser |
| Container (distroless, non-root, reproducible) | `Containerfile` |

OpenTelemetry SDK wiring, Prometheus registry, health-check registry and log
redaction are **not** re-implemented here by design.

---

## Quick start (template UX)

```bash
# 1. Use this template on GitHub, then:
git clone https://github.com/<you>/my-project && cd my-project

# 2. Configure (optional locally!):
cp .env.example .env          # APP_* and/or HELLNET_TELEMETRY_*

# 3. Verify everything works:
make test
make run                      # boots even without telemetry envs

curl -s localhost:8080/health
curl -s localhost:8080/metrics
curl -s 'localhost:8080/api/v1/hello?name=you'

# 4. Write business logic. That's your 20%.
```

No collector? Local structured logging and Prometheus `/metrics` remain active;
only remote OTLP export and profiling stay off. Add a
`HELLNET_TELEMETRY_ENDPOINT` to `.env` or the process environment to enable
remote logs, metrics, traces, and profiling without changing application code.

---

## Architecture

```text
                         HTTP
                          │
                ┌─────────▼──────────┐
                │  http.Server       │            ← hellnet-lib-api/server
                │  (timeouts, drain) │               Server + Run(ctx)
                └─────────┬──────────┘
                          │
        telemetry.Middleware(hellnet-lib-telemetry)    ← logs+metrics+traces for ALL routes
                          │
                ┌─────────▼──────────┐
                │   Gin Adapter      │            ← hellnet-lib-api/adapter
                │ req-id│sec-headers │               gin-backed api.Router
                │ cors │ recovery    │
                └─────────┬──────────┘
                          │ implements api.Router
                ┌─────────▼──────────┐
                │  API Abstraction   │            ← hellnet-lib-api/api
                │ Handler/Request/   │               transport-neutral contracts
                │ Response/errors    │
                └─────────┬──────────┘
                          ▼
                       Handler                       ← internal/<domain>/handler.go
                          ▼
                       Service
                  ┌──────┴──────┐
             Repository     External API           ← wire tel.HealthRegister /
                                                  tel.HTTPClient when added

Observability:  API ─► gin middleware ─► hellnet-lib-telemetry ─► Logs │ Metrics │ Traces
Lifecycle:      context ─► config ─► telemetry.New ─► deps ─► server
                        ⇄ signal ─► server.Shutdown ─► tel.Shutdown
```

### Why this layering pays off

1. **Swap-proof business code** — services/handlers never import `gin`; they
   see `hellnet-lib-api/api.Handler`, `Request`, `Response`, `*Error` only.
2. **Direct telemetry composition** — `cmd/api` initializes and wires
   `hellnet-lib-telemetry`; domain packages receive only the resulting logger
   and remain independent from telemetry construction.
3. **Honest testing** — tests exercise handlers through the abstraction port
   AND through the real Gin adapter, using only `httptest`.

---

## Environment variables

Two strict namespaces, zero overlap:

### Application (`hellnet-lib-api/config` reads these — `HELLNET_*`, fallback `APP_*`)

| Variable | Default | Purpose |
|---|---|---|
| `HELLNET_SERVICE` | **required** | Service identity exposed by the application; `FromEnv` fails if empty |
| `HELLNET_ENVIRONMENT` | `Development` | Logical environment label; log level is derived from it (`debug` in Development, `info` otherwise) |
| `HELLNET_PORT` | `8080` | Listen port |
| `HELLNET_SHUTDOWN_TIMEOUT` | `10s` | Drain budget; keep < k8s `terminationGracePeriodSeconds` |
| `HELLNET_READ_TIMEOUT` / `HELLNET_WRITE_TIMEOUT` / `HELLNET_IDLE_TIMEOUT` / `HELLNET_READ_HEADER_TIMEOUT` | `15s` / `30s` / `120s` / `10s` | Explicit `http.Server` hardening |
| `HELLNET_CORS_ALLOWED_ORIGINS` | *(disabled)* | Comma-separated exact origins or `*` |
| `HELLNET_BODY_LIMIT` / `HELLNET_LOG_FORMAT` / `HELLNET_TRUSTED_PROXIES` | see lib | Additional runtime knobs documented in hellnet-lib-api |

Full variable reference: <https://github.com/guilhermelinosp/hellnet-lib-api>

Build metadata (`version`, `commit`, `date`) arrives via `-ldflags`
(Makefile/Containerfile/CI) and appears at `GET /`.

### Telemetry — owned by hellnet-lib-telemetry

Documented with the library's own conventions; **never mirrored into `APP_*`**:

| Variable | Required | Purpose |
|---|---|---|
| `HELLNET_TELEMETRY_SERVICE` | required | Service identifier reported everywhere |
| `HELLNET_TELEMETRY_ENDPOINT` | required for remote export | OTLP base URL incl. port (e.g. `http://alloy.monitoring:4318`) |
| `HELLNET_TELEMETRY_ENVIRONMENT` | optional | Deployment environment resource attribute |

The library also accepts the legacy `HELLNET_*` names as fallback. With no
endpoint, OTLP export is disabled while stdout logs and `/metrics` stay active.

Full variable reference: <https://github.com/guilhermelinosp/hellnet-lib-telemetry>

---

## Observability

Everything below exists because the library does it natively:

* **Logging** — one structured logger: JSON to stdout *and* OTLP, correlated
  with `trace_id`. Inject `tel.Logger` into application dependencies.
  Do **not** add zap/zerolog/logrus.
* **Metrics** — HTTP instrumentation happens once around the whole router
  (`telemetry.Middleware(tel, handler)`): requests/duration/inflight/
  response+body size/error totals plus runtime (GC, memory, goroutines).
  `GET /metrics` serves Prometheus format from the same registry used for OTLP.
* **Tracing** — inbound spans extracted by the same middleware. For business
  operations that actually deserve a span:

```go
err := tel.WithSpan("orders.process", func(ctx context.Context) error {
    return s.repo.Create(ctx, order)
})
```

  Skip spans for trivial calls — signal-to-noise is a feature.
* **Health/readiness** — `/live` `/ready` `/health` are library handlers; the
  template only mounts them. Dependency checks plug in through
  [`HealthRegister`](#adding-an-observable-dependency-postgresql-redis-kafka).

---

## Project layout

```text
cmd/api/main.go              # tiny bootstrap: wiring + shutdown order ONLY
internal/
└── ride/                    # reference module (replace me!)
    ├── service.go           #   business rules behind Service interface
    └── handler.go           #   route declarations + input binding
openapi/openapi.yaml         # contract of the REAL endpoints (kept honest)
Containerfile · Makefile · .goreleaser.yaml · .github/workflows/*
```

The transport/abstraction layer (`api`, `adapter`, `config`, `server`) lives in
[hellnet-lib-api](https://github.com/guilhermelinosp/hellnet-lib-api) — it is
not duplicated here.

---

## Development

```bash
make run                 # zero-config local boot
make test                # race + shuffle, stdlib-only stacks
make cover-html          # coverage report
make lint                # golangci-lint
make build               # bin/golang-api-template with ldflags metadata
make image               # podman build (works with docker too)
./bin/golang-api-template --version-check via curl localhost:8080/
```

Tests run fast and deterministically without an OTLP collector: the library
keeps telemetry local when the endpoint is empty.

### Testing philosophy

* Abstraction behaviors (`errors` mapping, request/response ports) tested
  without any framework — the suite comes from hellnet-lib-api.
* Adapter integration behavior (routing, recovery, headers, CORS, 404/405
  envelopes) tested through `httptest` against the real Gin engine.
* Rendering live-socket graceful drain is covered by hellnet-lib-api
  `server` tests rather than duplicated here.

---

## OpenAPI

Contract lives at [`openapi/openapi.yaml`](openapi/openapi.yaml) documenting
the real surface: root discovery, probes, metrics and `/api/v1/hello` flavors.
Hand-maintained **but enforced**: every endpoint change should touch it in the
same PR (keeping docs honest beats generating drift).

---

## CI / CD

| Workflow | Trigger | Contents |
|---|---|---|
| `pr-check` | PR | shellcheck · merge strategy/conventional commits · gitleaks · labeler · **go-quality** (tidy guard/vet/race tests/build/lint/govulncheck/dependency-review) |
| `codeql` | PR→main | CodeQL security+quality (Go, actions) |
| `pipeline` | push main | org release (semver tag + GH Release) → go-quality → goreleaser artifacts/checksums upload → container image |

Every job is an import from [ci-templates](https://github.com/guilhermelinosp/ci-templates) — this repository owns zero CI logic, only flow declarations. Reuse the same two files in any Go service.

Releases trigger on tag push (created by the org `release` workflow with
semver derived from conventional commits). GoReleaser ships archives +
checksums for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64,
windows/amd64 onto that same tag's release page.

---

## Container

`Containerfile`: multi-stage (golang-alpine → distroless static), cache-mounts
for modules/build cache, `-trimpath`, ldflags-injected metadata, non-root user,
single static binary. Dev tools are absent from the runtime image by design.

---

## Recipes

### Creating a new API from this template

1. `Use this template` on GitHub → clone.
2. Search-and-replace module name `guilhermelinosp/golang-api-template` in
   `go.mod` + imports.
3. Rename `HELLNET_TELEMETRY_SERVICE` value wherever you configure it
   (`.env.example`, your deployment platform of choice).
4. Delete `internal/ride/*` (or your domain), its registration lines in `cmd/api/main.go`,
   and the `/api/v1` paths in `openapi/openapi.yaml`.
5. `make test && make run` → green baseline restored.
6. Build your first domain module following the recipe below.

### Adding an endpoint (the whole ceremony)

```go
// internal/orders/handler.go
package orders

import (
    "context"
    "net/http"

    "github.com/guilhermelinosp/hellnet-lib-api/api"
)

type CreateInput struct{ SKU string `json:"sku"` }

func (h *Handler) create(ctx context.Context, req api.Request) (api.Response, error) {
    var in CreateInput
    if err := req.Bind(&in); err != nil {          // strict JSON → 400/413 handled
        return api.Response{}, err
    }
    order, err := h.service.Create(ctx, in.SKU)    // pure ctx flow: no gin anywhere
    if err != nil {
        return api.Response{}, err                 // *api.Error → predictable envelope
    }
    return api.JSON(http.StatusCreated, order), nil
}

// Route declarations (path wildcards use web syntax):
func (h *Handler) Routes() []api.Route {
    return []api.Route{
        {Method: http.MethodPost, Path: "/orders", Handler: api.HandlerFunc(h.create)},
        {Method: http.MethodGet, Path: "/orders/{id}", Handler: api.HandlerFunc(h.get)},
    }
}
```

Wire it — three lines in `cmd/api/main.go`:

```go
ordersHandler := orders.NewHandler(orders.NewService(repo))
depsRoutes := append(helloHandler.Routes(), ordersHandler.Routes()...)
api.RegisterPlatform(router, info, api.Deps{Routes: depsRoutes})
```

That single registration yields: routing under `/api/v1`, request-id/security
headers/CORS/recovery, request logs+metrics+traces, validation/error envelopes,
request-size enforcement. Update `openapi/openapi.yaml` alongside.

### Adding an observable dependency (PostgreSQL, Redis, Kafka)

There is **no template-level health framework** — register checks directly
with the library; readiness reflects them immediately:

```go
// cmd/api/main.go, after opening your dependency
tel.HealthRegister("postgres", func(ctx context.Context) error {
    return sqlDB.PingContext(ctx) // readiness flips 503 automatically if down
})

// Outbound HTTP gets tracing/metrics for free by borrowing the lib client:
client := tel.HTTPClient(&http.Client{Timeout: 5 * time.Second})
```

Notes worth knowing:

* checks receive the `ctx` from the library on each probe run;
* the lib exports `healthcheck_status` gauges → visible at `/metrics` and OTLP;
* Kafka/cache equivalents follow the identical pattern with a cheap ping call;
* need an in-code DB pool? the library ships `WatchDB(db *sql.DB, "postgres")`.

---

## Deliberately NOT included (KISS/YAGNI)

DI frameworks · config libraries like Viper · second logging/metrics/tracing
stacks · parallel health framework · CQRS/event-bus/command-bus · repository
frameworks · generated OpenAPI pipelines · separate admin port · per-signal
OTLP toggles (the library exposes one switch by design) · validator libraries
(strict decoding covers template needs until proven otherwise).

Every omission has an owner: add dependencies only after
stdlib → hellnet-lib-telemetry → gin all fail to solve the problem.

---

## Contributing & License

See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md).
Licensed under [Apache 2.0](LICENSE).

<!-- release pipeline verification -->

<!-- verify round 2 -->

<!-- verify round 3 -->

<!-- verify round 4 -->

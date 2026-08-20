# [Project name]

_Replace the heading above with the project's name, and this line with one sentence describing what this app does for users._

## Run & Operate

- `cd backend-go && go run ./cmd/api` — run the production Go API server (port 8080)
- `cd backend-go && go test ./...` — run all Go unit & integration tests
- `pnpm run typecheck` — typecheck frontend & shared packages
- `pnpm run build` — build frontend & shared packages
- `pnpm --filter @workspace/api-spec run codegen` — regenerate API hooks from the OpenAPI spec
- Required env: `DATABASE_URL` — Postgres connection string

## Stack

- Frontend: React 19, TypeScript 5.9, Vite, TailwindCSS
- Backend: Production Go 1.24 API Server (Chi router, pgxpool, Slog)
- DB: PostgreSQL 16 + Embedded Migration Engine
- API Spec & Codegen: OpenAPI 3.0 + Orval (React Query hooks)
- Container: Docker (Multi-stage non-root runtime)

## Where things live

_Populate as you build — short repo map plus pointers to the source-of-truth file for DB schema, API contracts, theme files, etc._

## Architecture decisions

_Populate as you build — non-obvious choices a reader couldn't infer from the code (3-5 bullets)._

## Product

_Describe the high-level user-facing capabilities of this app once they exist._

## User preferences

_Populate as you build — explicit user instructions worth remembering across sessions._

## Gotchas

_Populate as you build — sharp edges, "always run X before Y" rules._

## Pointers

- See the `pnpm-workspace` skill for workspace structure, TypeScript setup, and package details

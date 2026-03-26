# Self-Service Provider Project Routes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move provider/project frontend traffic off `/api/admin` onto tenant-scoped self-service APIs so authenticated non-admin users can manage their own tenant data without hitting admin RBAC.

**Architecture:** Add a thin HTTP handler under `/api/providers` and `/api/projects` that reuses existing `AdminService` tenant-scoped methods, then point frontend provider/project transport methods at these new endpoints. Keep the existing `/api/admin` handler unchanged for true admin resources.

**Tech Stack:** Go `net/http`, existing `AdminService`, React Query, Axios transport

---

### Task 1: Lock regression with backend tests

**Files:**
- Create: `internal/handler/self_service_test.go`

- [ ] **Step 1: Write the failing test**

Add handler tests that prove:
- member `GET /providers` returns tenant providers
- member `GET /projects` returns tenant projects
- member `GET /projects/by-slug/{slug}` returns the matching tenant project

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler -run SelfService`
Expected: FAIL because self-service handler/routes do not exist yet.

- [ ] **Step 3: Write minimal implementation**

Create a dedicated self-service handler for providers/projects using existing service methods and tenant context.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler -run SelfService`
Expected: PASS

### Task 2: Register tenant-scoped routes

**Files:**
- Modify: `internal/core/database.go`
- Modify: `internal/core/server.go`

- [ ] **Step 1: Wire handler into server components**

Add the new handler to `ServerComponents` and initialize it next to `AdminHandler`.

- [ ] **Step 2: Register `/api/providers` and `/api/projects`**

Wrap with existing auth middleware when auth is enabled; otherwise reuse `NoAuthMiddleware`.

- [ ] **Step 3: Re-run backend tests**

Run: `go test ./internal/handler -run SelfService`
Expected: PASS

### Task 3: Switch frontend provider/project transport paths

**Files:**
- Modify: `web/src/lib/transport/http-transport.ts`
- Modify: `web/src/hooks/queries/use-settings.ts`
- Modify: `web/src/pages/providers/index.tsx`

- [ ] **Step 1: Point provider/project methods at `/api/providers` and `/api/projects`**

Move list/detail/create/update/delete and provider export/import off the admin Axios base URL.

- [ ] **Step 2: Prevent member provider page from fetching admin-only settings**

Guard the provider page settings query/UI so member users do not trigger `/api/admin/settings` on page load.

- [ ] **Step 3: Run frontend verification**

Run: `pnpm build` in `web/`
Expected: build succeeds

### Task 4: Final verification

**Files:**
- No additional files

- [ ] **Step 1: Run targeted Go verification**

Run: `go test ./internal/handler -run SelfService`
Expected: PASS

- [ ] **Step 2: Run frontend build**

Run: `pnpm build` in `web/`
Expected: PASS

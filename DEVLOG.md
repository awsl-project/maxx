# DEVLOG

## 2026-07-16

- Implemented available-only model list semantics for maxx model-list endpoints.
- `/v1/models` now filters collected candidates through the current routing scope instead of exposing historical/pricing/provider candidates directly.
- `/project/{slug}/v1/models` respects project route scope.
- `/provider/{id}/v1/models` constrains results to the requested provider.
- Cooldown, disabled route, provider adapter availability, client type, project, and token context are covered by regression tests.
- Generated screenshot evidence for global, provider-scoped, project-scoped, and external mock/cooldown interaction nodes.
- Converted the providers bulk delete/select toolbar from a normal top-of-list block to a sticky toolbar inside the providers scroll container.
- Added a targeted layout regression test asserting sticky/top/z-index/wrapping/readable surface classes.
- Decoupled committed stream read EOF retry from `DisableErrorCooldown` so provider/network stream EOFs can follow route retry policy without requiring cooldown opt-out.
- Added regression coverage for committed stream EOF retry with cooldown enabled/disabled, zero retry budget, client cancellation, and custom adapter unexpected EOF classification after response start.

# Magpie redaction engine

Source: https://github.com/yetone/magpie/tree/7621820/internal/redact

Copyright (c) 2026 yetone. MIT license, retained in `LICENSE`.

Maxx adaptation: mutable package state and the functions that depend on it
are instance methods on `State`; `Options.State` and `Writer.scopeState` carry
the selected instance. This isolates reversible mappings, learned values,
shape substitutions and memoization by tenant and project. The legacy package
functions delegate to a default state to run the upstream regression suite;
Maxx runtime integration must use explicit state instances.

Upstream unit cases retain their assertions. Their direct global-state resets
target the default state, and the single Claude-seal fixture is generated locally
with the same public framing. The upstream app-specific test harness and external
PII-model evaluation entry point are not copied here; Maxx tests model integration
at the privacy-module boundary.

The Maxx runtime limits each scope to 4,096 mappings / 8 MiB and bounds scope
retention. Request-local restoration views contain only mappings used by that
exchange. Optional integration hooks inspect explicitly named credential fields
and nested JSON, capture matches for archive filtering, and apply the project
default action. Binary response bodies pass through unchanged.

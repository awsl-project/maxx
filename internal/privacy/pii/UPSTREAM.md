# Local privacy model support

Adapted from https://github.com/yetone/magpie/tree/7621820/internal/pii.
Copyright (c) 2026 yetone. The MIT license is retained in `LICENSE`.

Maxx changes the cache namespace, preserves the inherited HTTP proxy and CA trust,
and uses small platform-specific command helpers. No model is downloaded or
started automatically. The global policy explicitly selects a model; public model weights
and backend processes may be shared, while reversible originals remain isolated
in the enclosing Maxx privacy runtime. Model cache files contain only SHA-256 text
hashes and span metadata, never original text.

- AstrLink Guard: QuantumNous/astrlink-guard, revision
  `7216b6a3b897ef903d4b1ba367ae6fb95b0fa5e4`, Apache-2.0. The derived CoreML
  graph in `guard/coreml.mlmodel` / `coreml.json` follows the same license;
  its license text is in `guard/LICENSE.model`. Model weights/tokenizer are
  downloaded separately and checked against the SHA-256 values in `astrlink.go`.
- Privacy Filter: OpenAI's Apache-2.0 model, LocalAI-io's q8 GGUF conversion at
  revision `935d86882f4b5dc6e0eef05aae48ae5ba0a82a7a`. The executable is Magpie's
  separately downloaded `pf-v2` helper, built from privacy-filter.cpp/ggml;
  both assets are checked against the hashes pinned in `pii.go`. Neither the
  helper nor these model weights are committed to Maxx.

Upstream backend/lifecycle/download tests remain, with an isolated test cache and
local fake helper. `MAXX_PRIVACY_TEST_MODELS=astrlink-guard,privacy-filter` runs
explicit real downloads/inference; ordinary test runs stay off the network.

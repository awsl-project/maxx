# Project privacy protection

Privacy is an independent module (`internal/privacy`) and settings page
(`/privacy`). Projects opt in; new and existing projects default to **off**.
When off, requests skip detection, replacement, restoration and new archive
filters. Existing forwarding, retry, authentication, billing, detail retention,
sensitive-header protection and global logging behavior are preserved.

## Configure a project

Open **Privacy Protection → Configure** for a project. Edit its policy, then
**Save privacy policy**. The next HTTP request or WebSocket turn uses the new
policy; an in-flight request keeps its original policy snapshot.

- **Mask and restore:** send placeholders (or supported shape-preserving fake
  keys/usernames) upstream, then restore values in client-facing responses.
- **Warn:** record findings but send the original content. This intentionally
  exposes those values to the provider.
- **Block:** return `400 privacy_blocked`, record the request as `REJECTED`, and
  create no upstream attempt for the rejected payload.
- Per-kind actions override the default: e.g. `EMAIL → mask`, `API_KEY → block`.
  A rule's uppercase kind can be used here too.

The compact project-list switch remains compatible with the earlier
`blockSecrets` API. A project without a saved full policy retains credential
**blocking** when enabled through that switch. Saving the detailed form selects
its explicit action; switching it off/on subsequently preserves those settings.
The general project editor preserves module settings. Only authenticated project
administrators can access the full policy, previews, records or original values.

## Detection and custom rules

Credential rules cover recognized provider keys, JWTs, private keys, credential
assignments, connection passwords and explicitly named credential fields in
native/embedded tool JSON. Optional personal-data rules cover email, phone,
identity and payment cards, SSN, passports, IBAN, birthdays, MAC addresses and
serial numbers. User directories, usernames/hostnames, IPs and bucket names are
separately selectable. Semantic names/addresses require a local model.

Custom keywords support case-insensitive and whole-word matching (one value per
line, at most 200). Custom credential rules accept **one prefix or RE2 regex**:

```json
{
  "kind": "CORPORATE",
  "prefix": "corp_"
}
```

```json
{
  "kind": "CUSTOM_TOKEN",
  "regex": "internal_token=([A-Za-z0-9_-]{8,})"
}
```

A regex's first capture group is replaced when present, otherwise its complete
match. Matches must contain at least four characters. At most 32 rules; prefixes
are 3–64 characters and regexes at most 300 characters. Custom credential rules
follow **Credential detection**. Keywords also work without credential/PII rules.
The allowlist matches complete values, case insensitive; allowed values are sent
unchanged. Preview applies the current editor draft, without forwarding, changing
project policy, adding history or learning new originals.

Protocol IDs, model/tool names, schema/grammar structure, signatures, encrypted
reasoning and opaque media stay intact. Tool arguments/results are treated as
user content. Text document payloads are inspected; binary file parts are not.
Tool/schema descriptions use rules and known values without expensive model
inference. Chinese-name and linker-symbol false-positive guards follow Magpie.

## Reversible values and transports

Rules/replacement behavior is adapted from Magpie
[`7621820`](https://github.com/yetone/magpie/tree/7621820), with its MIT notices
and upstream regression tests retained. Mutable state is scoped by tenant and
project within one Maxx runtime. Mapping caches are bounded (4,096 entries /
8 MiB per active scope; at most 16 cached scopes). Learned values may expire
with eviction or clearing. Key files preserve placeholder identity across
restarts when the same data directory is retained; originals are never in them.

A response's restoration view contains **only values replaced in that request**.
Other projects, tenants and unrelated previous requests cannot reveal originals
by returning their placeholders. JSON/tool payloads, HTTP/SSE (including split
placeholders) and native Responses WebSocket frames use the same restoration
engine. Binary responses pass through. Restore/write failures remain client
transport failures rather than provider cooldown events. Conversion/capture,
retry, provider selection and billing keep their existing responsibilities.

Provider exceptions bypass forwarding protection only when **all possible
providers in the resolved fallback plan** are exempt. A mixed plan is inspected.
Archives still scrub exempt requests; model-only content is omitted when model
inspection was bypassed. Backup references use provider names and are remapped
on import; missing names remove exceptions rather than trusting source IDs.

## Local models

Select **AstrLink Guard** (~39 MB) or **Privacy Filter** (~1.6 GB) and explicitly
download/load it. Downloads use pinned versions and SHA-256 checks, resume and
can be canceled. Weights are not included in Maxx. Threshold `0` uses the model
default; custom thresholds must be at least `0.5` and below `1`.

The master project switch defaults off. Maxx does not copy Magpie's automatic
model activation/download defaults. Once a project explicitly enables model
inspection, unavailable/failed inference returns `503 privacy_check_failed`;
requests never silently fall back to rules alone. Weights/processes and hashed
span caches are public shared resources; only default-tenant server
administrators can download, cancel, load, unload or remove them. Other project
administrators can select installed models. Unloading/removing a shared model
can affect other opted-in projects; they fail closed until it is available again.

Model cache files contain SHA-256 hashes and span positions/labels/scores, not
request text. Model provenance/licenses are in `internal/privacy/pii/UPSTREAM.md`.
`MAXX_PRIVACY_CACHE_DIR` overrides the model cache directory.

## Archives and detection history

For opted-in requests, new database snapshots and request-error broadcasts
remove credentials, selected static personal/custom matches and values observed
by the model (including warn-only findings). Archives never run inference in a
DB write. SSE/JSONL text is checked across deltas; unsafe bodies may be omitted
with a redaction notice. Archive processing copies objects and does not modify
provider/client payloads. Existing rows and global log files are not rewritten;
use existing retention/cleanup controls for historical data. Global runtime logs
retain existing behavior because they lack reliable project context.

The history tab provides pagination, kind/action/finder/provider/time filters,
details, restored counts and aggregate counts. Disk retains **metadata only**
under `<data-dir>/privacy/<tenant>/<project>/records`, for at most 30 days and
100 MiB per project. Original values and processed sent bodies are memory-only,
bounded to 400 recent records and 16 MiB per active project scope (at most 4 MiB
per record); they disappear after eviction/restart/clear. Sent bodies show the
privacy module's processed request, before adapter-specific wire preparation.
Record clearing also forgets learned originals and fences in-flight record writes.
Policy/configuration backups remain sensitive: custom keywords and allowlists
are administrator-supplied settings, not detection history.

## Verification

The module includes upstream rule, replacement, model/cache/download/helper
regressions plus Maxx tests for scope isolation, disabled behavior, actions,
custom rules, allowlists, native/embedded JSON, multipart binary preservation,
split SSE/WebSocket restoration, failures/cancellation, archived copies, memory
bounds and backup references. End-to-end tests exercise the real Maxx pipeline
with controlled upstreams, and Playwright checks settings/preview/history on the
built server. Screenshots use synthetic content and are never committed.

Real model downloads/inference can be verified explicitly on the host:

```sh
MAXX_PRIVACY_TEST_MODELS=astrlink-guard,privacy-filter \
  go test -count=1 -timeout=20m ./internal/privacy/pii -run TestRealLocalModels -v
```

Ordinary CI uses deterministic local helper/tokenizer/kernel fixtures and does
not download gigabytes of model assets.

# Credential protection

Maxx uses stateless rules inspired by [Magpie](https://github.com/yetone/magpie)
([MIT attribution](../internal/privacy/LICENSE.magpie)). There is no external
service, local AI model, reversible placeholder map, or shared secret cache.

## Stored request details

For projects with privacy protection enabled, new request and upstream-attempt
snapshots are scrubbed before database writes:

- Sensitive headers, including custom `*-token` / `*-secret` headers, are
  scrubbed. URL passwords and credential query parameters are scrubbed too,
  including percent-encoded values. Malformed URLs are omitted from the archive.
- JSON credential fields (such as `password`, `api_key`, and `access_token`) are
  replaced with `[REDACTED]`. Recognized key formats, JWTs, PEM private keys,
  quoted/unquoted credential assignments, and connection-string passwords are also scrubbed
  from textual bodies and error records.
  Unquoted code references and function calls (such as `api_key = config.api_key`)
  are excluded from assignment matching; quoted literal credentials remain protected.
  Existing `[REDACTED]` values are safe to reuse in tool results without triggering a new block.
- JSON embedded in tool-result strings is inspected too. JSON numbers retain
  their precision, and base64 image/audio/video data URLs remain opaque.
- SSE and JSONL/WebSocket archives are checked across text deltas. If a
  credential is detected in their combined text, the stored body is omitted
  with a redaction notice. Usage, status, timing and billing metadata remain.
- Developer mode does not bypass opt-in archive scrubbing. Request-error
  broadcasts for protected requests also scrub recognized patterns. Global runtime
  logs retain existing behavior; they have no reliable project context.

This changes archived copies only. It does not change a response delivered to
clients, provider configuration, or provider/API-token backups. Existing database
rows and log files are not rewritten; use the existing retention/cleanup controls
for historical details and protect configuration backups separately.

## Block credentials before sending upstream

Open **Privacy Protection** (中文：**隐私保护**) from the sidebar and enable
credential blocking for the projects you want to protect. Changes save immediately.
A project’s Overview links directly to its privacy settings. This switch controls
both outbound blocking and new archive scrubbing. It defaults to **off** for both new
and existing projects, is tenant scoped, and is preserved in configuration backups.

When enabled, Maxx checks message text, system instructions, prompts and tool
results/arguments before contacting a provider. A recognized credential returns
HTTP 400 with `privacy_blocked`; the request is recorded as `REJECTED`, without an
upstream attempt, retry, cooldown or charge. Each WebSocket turn is checked. The
same policy applies to direct `/provider/{id}/...` requests when a project is
selected. Requests without a project and projects with protection off bypass body inspection
and the new archive rules. The existing sensitive-header redaction remains.
Changing the setting affects the next HTTP request or WebSocket turn; an in-flight
request keeps the policy captured when it entered the executor.

Actual authentication headers, model names, protocol IDs, opaque signatures and
tool definitions are outside the outbound check. Allowed request bytes are not
rewritten. Multipart text fields are checked, while file parts and embedded
image/audio data are not inspected. Unsupported or malformed bodies are rejected
only when the switch is on; policy lookup failures do not silently disable protection.

For example, this message is blocked with the switch on:

```json
{"model":"gpt-4o","messages":[{"role":"user","content":"API_KEY=example_example_example"}]}
```

The privacy settings API accepts `PUT /api/projects/{id}/privacy` with
`{"blockSecrets": true}` (or `false`). It requires an administrator in the project’s
tenant and changes only privacy policy; names, slugs and routes come from current
server data. The legacy project API remains compatible and preserves the policy
when an older editor omits `blockSecrets`.

## Scope

This is a credential safeguard, not complete personal-data detection or a DLP
boundary. Rules can miss unknown formats or obfuscated secrets and can flag
examples that resemble credentials. Names, addresses, email, IP addresses,
passwords without recognizable context, encoded attachments and OCR are outside
this first version. No content is sent to a separate detection service.

## Magpie comparison

This iteration was checked against Magpie
[`ee6f8866`](https://github.com/yetone/magpie/tree/ee6f8866f33165eebfd16a7e3f6948e04132096e),
on 2026-10-28, particularly its credential rules, opt-out fast path and irreversible
archive scrubber. New regression cases cover natural-language CJK credential
contexts (including numeric keys versus quantities), Redis URLs without a username,
relay API keys and SK hynix article slugs. Maxx does not adopt Magpie’s global known-secret
cache or reversible mapping.

| Area | Maxx in this PR |
| --- | --- |
| Recognized formats | OpenAI/Anthropic and relay keys, GitHub/GitLab, Google/AWS, Slack/Stripe, Hugging Face/Groq/xAI, npm/PyPI, DigitalOcean/SendGrid, Maxx tokens, JWT and PEM/PGP private keys. |
| Password context | Quoted values retain punctuation and escapes; unquoted code references and variable expansions are excluded. AWS and database credential fields are recognized. Named CJK password/key contexts and Redis URLs without usernames are checked. |
| Archive protection | JSON, nested tool JSON, SSE/JSONL fragments, URL credentials, headers and error records are irreversibly scrubbed. No credential values are cached. |
| Upstream behavior | Project-level blocking. Magpie's reversible masking and response restoration are outside this PR. |
| Personal data / custom rules | Magpie supports additional options; this PR covers credential protection only. |

## Module boundaries

`internal/privacy` owns the feature:

- `rules.go` / `named.go`: credential patterns, natural-language contexts and text detection/redaction.
- `inspect.go`: request content parsing and inspection.
- `archive.go`: irreversible body, URL, header and snapshot redaction.
- `service.go`: tenant-scoped project policy, settings updates and request decisions.

The executor receives a privacy service at startup and records rejected requests.
The router contains no privacy policy logic. Storage and broadcast adapters
call the module only for protected requests to scrub copies. The executor carries
a private request-scoped flag into requests and attempts; it is never accepted from
JSON and adds no archive database column. Both CLI and desktop runtimes use the same cached
project store for policy reads and writes.

`web/src/pages/privacy` provides the standalone settings page; project pages show
status and link to it. The page and settings API require administrator access.

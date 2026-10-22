# Credential protection

Maxx uses stateless rules inspired by [Magpie](https://github.com/yetone/magpie)
([MIT attribution](../internal/privacy/LICENSE.magpie)). There is no external
service, local AI model, reversible placeholder map, or shared secret cache.

## Stored request details

New request and upstream-attempt snapshots are scrubbed before database writes:

- Existing sensitive-header redaction remains in place.
- JSON credential fields (such as `password`, `api_key`, and `access_token`) are
  replaced with `[REDACTED]`. Recognized key formats, JWTs, PEM private keys,
  credential assignments, and connection-string passwords are also scrubbed
  from textual bodies and error records.
- JSON embedded in tool-result strings is inspected too. JSON numbers retain
  their precision.
- SSE and JSONL/WebSocket archives are checked across text deltas. If a
  credential is detected in their combined text, the stored body is omitted
  with a redaction notice. Usage, status, timing and billing metadata remain.
- Developer mode does not bypass archive scrubbing. Runtime logs emitted through
  Maxx's log writer and request-error broadcasts also scrub recognized patterns.

This changes archived copies only. It does not change a response delivered to
clients, provider configuration, or provider/API-token backups. Existing database
rows and log files are not rewritten; use the existing retention/cleanup controls
for historical details and protect configuration backups separately.

## Block credentials before sending upstream

Open **Projects → project → Overview**, enable **Block credentials in messages**
(中文：**拦截消息中的凭据**), and save. The switch defaults to **off** for both new
and existing projects, is tenant scoped, and is preserved in configuration backups.

When enabled, Maxx checks message text, system instructions, prompts and tool
results/arguments before contacting a provider. A recognized credential returns
HTTP 400 with `privacy_blocked`; the request is recorded as `REJECTED`, without an
upstream attempt, retry, cooldown or charge. Each WebSocket turn is checked. The
same policy applies to direct `/provider/{id}/...` requests when a project is
selected. Requests without a project only receive archive scrubbing.

Actual authentication headers, model names, protocol IDs, opaque signatures and
tool definitions are outside the outbound check. Allowed request bytes are not
rewritten. Multipart text fields are checked, while file parts and embedded
image/audio data are not inspected. Unsupported or malformed bodies are rejected
when the switch is on; policy lookup failures do not silently disable protection.

For example, this message is blocked with the switch on:

```json
{"model":"gpt-4o","messages":[{"role":"user","content":"API_KEY=example_example_example"}]}
```

The project API accepts `blockSecrets: true` / `false` on create or update. Updates
that omit the field preserve the current policy so an older route editor cannot
silently turn it off.

## Scope

This is a credential safeguard, not complete personal-data detection or a DLP
boundary. Rules can miss unknown formats or obfuscated secrets and can flag
examples that resemble credentials. Names, addresses, email, IP addresses,
passwords without recognizable context, encoded attachments and OCR are outside
this first version. No content is sent to a separate detection service.

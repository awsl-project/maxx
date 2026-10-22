# WorkBuddy AI (international)

Create a **WorkBuddy AI (International)** provider, start browser login, open the returned HTTPS link on your own device, complete authorization, and select **Check authorization**. Save the provider to persist credentials. This URL-and-poll flow works on a headless/server deployment; it does not require a localhost callback. Manual credentials are also supported; expiration timestamps are Unix milliseconds.

Use **Discover available models** to query the account’s `/v3/config` CLI model list. Team accounts send both enterprise and tenant headers. Model IDs are not hard-coded and points are not treated as dollar prices.

The upstream is `https://www.workbuddy.ai/v2/chat/completions` and always receives `stream: true`. maxx forwards streaming replies and aggregates text/tool deltas for non-streaming callers. Other client protocols use maxx’s existing executor conversion. Only chat completions are supported natively; images/audio/Responses endpoints are not native capabilities.

Refresh is demand-driven with concurrent renewal sharing, short-lived old-token result reuse, and copy-on-write persistence through the router provider-update callback. There is no check-in or points scheduler. Login sessions and refresh sharing are process-local: pin browser login start/poll to one instance and keep each WorkBuddy account on a single credential-refresh owner. Concurrent rotation of shared credentials across independent instances is not coordinated by a distributed lock and is not supported.

User/system content and tool history are preserved. The adapter does not rewrite identities or move system prompts to bypass upstream channel restrictions. Upstream `unapproved channel`, policy, authentication, and quota errors remain failures; the adapter does not repeatedly retry restricted accounts.

Reference: [magpie-community WorkBuddy plugin](https://github.com/magpie-community/plugins/tree/main/packages/workbuddy), including its sign-in, refresh, enterprise, conversation, and tool tests. Only the authentication/API contracts are reused; prompt/identity rewriting behavior is intentionally excluded.

Validation uses local mock upstreams and actual executor dispatch. No real WorkBuddy account was available, so successful live browser login, plan permissions, and live model inference remain unverified.

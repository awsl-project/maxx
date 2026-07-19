# Contributing to maxx

Thanks for contributing! maxx is a **public** repository — please read the
sensitive-information policy below before you commit.

## Sensitive information — do not commit

Before every commit, make sure the diff contains **none** of the following,
whether in code, comments, tests, fixtures, docs, or example config:

- **Credentials / secrets** — API keys, tokens, passwords, or URLs with embedded
  auth. Use obviously-fake placeholders in tests and examples: `sk-test`,
  `"dummy"`, `https://example.com`, RFC 5737 IPs, AWS's documented
  `AKIAIOSFODNN7EXAMPLE`.
- **Private deployment details** — real hostnames, internal domains, or the
  staging/prod URLs and operator/customer names of any actual deployment.
- **Internal or private product names** — the name of a specific relay/gateway
  product or service that a deployment happens to use. Do **not** drop these into
  comments or test data as a casual "e.g. …" example. For an illustrative
  example, name a well-known **public** product (OpenAI, Anthropic, Ollama,
  New API, AWS Bedrock…) or use a generic placeholder such as `example-relay`.

Publicly documented, first-class provider **templates** (shipped in
`web/src/pages/providers/types.ts` with a logo, i18n copy, and preset URLs) are
an intentional, user-facing feature and are **not** covered by this rule.

### How this is enforced

- **CI (`Secret Scan` workflow)** runs [gitleaks](https://github.com/gitleaks/gitleaks)
  on every PR. It catches credential-shaped secrets — but **not** arbitrary
  internal names, which rely on this policy and reviewer diligence.
- Maintainers: enable GitHub **Secret scanning** + **Push protection** (repo
  Settings → Code security) for an extra push-time gate; both are free for
  public repos.

Quick local self-check before committing:

```sh
git diff --cached | grep -inE "sk-(ant|or)-|AKIA|password|secret|Bearer [A-Za-z0-9]"
```

If a secret or private name has already reached `main`, note that scrubbing the
current tree does **not** remove it from git history — flag it to the maintainers
so they can decide whether a history rewrite (e.g. `git filter-repo`) is
warranted, and rotate any exposed credential immediately.

## Build, lint & test

Backend (Go):

```sh
go build ./...
go vet ./...
gofmt -l .          # must print nothing
go test ./...
```

Frontend (`web/`, pnpm):

```sh
pnpm typecheck      # tsc -b
pnpm lint           # eslint
pnpm test           # vitest
pnpm build          # tsc -b && vite build  (produces web/dist, embedded by Go)
```

Run the checks relevant to what you touched; CI runs the full set (Backend,
Frontend, e2e, multiinstance, playwright, Secret Scan).

## Conventions

- Match the style of the surrounding file (naming, comment density, idioms).
- New provider types are registered via `provider.RegisterAdapterFactory` and
  wired in as blank imports; mirror an existing first-class adapter
  (`openrouter`, `newapi`, `ollama`) rather than adding discriminator flags to
  the generic `custom` adapter.
- Model mapping is data (ModelMapping entities via `executor.mapModel`), not
  inline config maps.

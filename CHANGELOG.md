# Changelog

Notable changes in this fork, newest first. Upstream's history lives at
[ejagombar/FireflyIII-AI-Categorizer](https://github.com/ejagombar/FireflyIII-AI-Categorizer).

## 2.0.0 — 2026-11-07

Placeholder payees are recognised instead of being used as merchants, and
merchant names can optionally be learned and remembered.
Image: `ghcr.io/lucasnasc2/fireflyiii-ai-categorizer:2.0.0` (also `2.0`, and
`latest` while `main` stays here).

### Recognising placeholder payees (upstreamable — PR #8)

- `classifier.IsBlankName` treats empty names, `unknown…`, parenthesised names
  and the known placeholders as "no merchant", so `(unknown destination)` no
  longer becomes a history group key. Write-up: [UPSTREAM.md](UPSTREAM.md).
- History is keyed on the description, which is what upstream already does for
  nameless payees.
- The prompt carries the raw line once, plus a note that the payee was not
  recorded and the merchant has to be read from the description.
- Destination matching filters placeholder accounts out, in both directions.

### Learned merchant names (opt-in)

- New setting **Settings → Behaviour → Learn merchant names from
  descriptions**, or `MERCHANT_LOOKUP_ENABLED=true`. Off by default.
- When on, the model may name the merchant for a transaction with no usable
  payee. The name is remembered in memory, keyed by a letters-only fingerprint
  of the description, and later repeats of that description are classified from
  history without a model call.
- Names are trimmed, blank or longer-than-60-rune names are discarded, and
  nothing is ever written to Firefly III. The memory is lost on restart.

### Operating the container

- `DRY_RUN=true` runs the whole decision path and logs every outcome without
  writing to Firefly III.
- `docker-compose.yml` runs the published image (`AICAT_TAG`, default
  `latest`), publishes on loopback only, and bind-mounts `./data` so saved
  settings survive container recreation.
- `Dockerfile` gains a `HEALTHCHECK` on `/health`, so plain `docker run`
  reports readiness too.
- CI: `go vet` and `go test` gate publishing, pull requests now run the tests,
  and superseded runs are cancelled.
- Validation tooling: `tools/audit.py` (unknown-payee rate, AI tags, category
  spread) and `tools/dryrun.py` (stub provider, full prompt log, `--merchant`).

## 1.0.0 — 2026-11-07

First fork build, published as `:1.0.0` / `:1.0`. It contained a bank-specific
merchant parser (`internal/classifier/merchant.go`, `tools/parsedump`) that has
since been removed in favour of the generic placeholder handling above, so
**`1.0.0` should not be deployed** — use `2.0.0`.

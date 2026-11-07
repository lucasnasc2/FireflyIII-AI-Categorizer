# Recognise placeholder payees instead of treating them as merchants

> Ready to submit as the body of an issue or pull request against
> [ejagombar/FireflyIII-AI-Categorizer](https://github.com/ejagombar/FireflyIII-AI-Categorizer).
> Copy it from this account or any other.


Some banks never send a `destination_name`. Firefly III then stores the card
purchase with the payee `(unknown destination)`, and that string became the
history group key: `isBlankName()` knows `(no name)`, `unknown` and `n/a`, but not
`(unknown destination)`.

Because history is grouped by payee, every unrelated shop that arrived this way
shared a single bucket. Whichever category had been decided for one merchant was
then offered to the model as the history for the next one - and reused. In my
data **50.7% of September and 89.5% of October withdrawals** carried that
placeholder, so most card spending was being categorised from one shop's history.

## Fix

Make the placeholder check generic, so it does not depend on knowing which
strings a particular bank writes:

```go
// IsBlankName reports whether a payee name identifies no merchant: it is empty,
// a known placeholder, wrapped in parentheses (Firefly III's convention for a
// placeholder account), or mentions "unknown".
func IsBlankName(s string) bool {
    norm := strings.ToLower(strings.TrimSpace(s))
    if blankNames[norm] {
        return true
    }
    if strings.Contains(norm, "unknown") || strings.Contains(norm, "no name") {
        return true
    }
    return strings.HasPrefix(norm, "(") && strings.HasSuffix(norm, ")")
}
```

With that, `GroupKey` falls back to the description - which is what it already
does for nameless payees - and each merchant gets its own history. Real payees
are untouched: when Firefly III holds a merchant name it is still used as-is.

The parentheses rule and the short `blankNames` list also cover the placeholders
seen in the wild (`no registrado`, `POS Purchase`, `(sin nombre)`, ...) without
teaching the code any one bank's format.

## Two related places

The same assumption - "the payee is a merchant" - was present twice more:

- **Destination candidates.** Placeholder accounts were offered to the model as
  MATCH candidates, so a withdrawal could be filed under `(unknown destination)`.
  They are now filtered out, and a history-matched destination pointing at one
  is dropped.
- **The prompt.** A blank payee caused the raw bank line to be printed in the
  "Destination:" slot, duplicating the "Description:" line right below it and
  inviting the model to echo that string back as an account name. The prompt now
  says the payee was not recorded and asks the model to identify the merchant
  from the description, in one line instead of two.

Before:

```
Transaction to classify:
  Destination: COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12
  Description: COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12
  Amount: 42.10
```

After:

```
Transaction to classify:
  Payee: not recorded by the bank — identify the merchant from the description
  Description: COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12
  Amount: 42.10
```

## Files

| File | |
|---|---|
| `internal/classifier/classifier.go` | +30/-8 - generic `IsBlankName`, wider `blankNames` |
| `internal/classifier/prompt.go` | +10/-7 - payee line, one rule per system prompt |
| `internal/pipeline/pipeline.go` | +15/-6 - placeholder accounts and history destinations |
| `internal/classifier/classifier_test.go` | new, 128 lines |
| `README.md` | +22 - *Payees that are not merchants* |

## Validation

There is no test workflow in this repo, so I ran these on this exact commit
(Go 1.22):

```
$ go vet ./...   # clean
$ go test ./...
ok  .../internal/classifier  0.006s
$ go build ./... # clean
$ docker build . # builds with the repository's Dockerfile
```

The test file is the repo's first, and uses only the standard library - six
functions covering the name check (26 placeholder and real-payee cases), the
regression itself (`GroupKey("(unknown destination)", descA) !=
GroupKey("(unknown destination)", descB)`), and the shape of the user prompt.

## Compatibility

- No config change, no new setting, no new environment variable.
- Behaviour only differs where the payee is a placeholder; those transactions
  now group on their description instead of on the placeholder.
- A real `destination_name` still wins over the description.

## Not in this PR

The placeholders themselves could be cleaned up at the source, but that is a
different trade: either the payee is left empty (the bank's truth) or a merchant
name is inferred from the description. Inferring it means asking the model for a
canonical merchant in the same response and caching `description -> merchant`, so
repeats can skip the call entirely; that is a behaviour change with a small
per-merchant token cost, so it belongs behind a setting and a separate PR. My
fork carries a bank-specific parser that does this offline, and it is not
proposed here: it only knows one bank's format, and the generic fallback above is
what a shared tool should do.

---

Written with AI assistance; validated against live Firefly III data.

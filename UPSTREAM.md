# Categorise on the merchant parsed from the description, not on the placeholder payee

*This file is a ready-to-submit issue / pull-request body for
[ejagombar/FireflyIII-AI-Categorizer](https://github.com/ejagombar/FireflyIII-AI-Categorizer).
Copy it into an issue, or use it as a PR description, from this account or any
other. The reference implementation is
[`lucasnasc2/FireflyIII-AI-Categorizer`](https://github.com/lucasnasc2/FireflyIII-AI-Categorizer)
at commit `8b958b6`, kept as a single small diff. It is offered freely under the
existing AGPL-3.0 licence, with no expectation that anyone maintains my fork.*

---

## Problem

Some banks never populate `destination_name`. Openbank (Spain) leaves the payee
empty — Firefly III shows it as `(unknown destination)` — and encodes
everything in the withdrawal description:

```
COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12
COMPRA BIZUM VENTA BILLETES RENFE 2026-08-25 PEDIDO 000457060982
TRANSFERENCIA A FAVOR DE LA CUEVA DEL BARBERO CONCEPTO: FACTURA 12
COMPRA EN WWW.AMAZON*WB1R672N5, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12
```

`classifier.GroupKey` keys the history lookup on the destination name, so every
one of those withdrawals shares the key `(unknown destination)`. The
consequences are worse than "slightly less accurate":

1. **History leaks across merchants.** The few-shot examples are meant to say
   "this is how earlier purchases from this vendor were categorised". Under a
   shared placeholder they say "how earlier purchases from *any* vendor were
   categorised", so a category decided once for a bar is offered back to the
   model as precedent for a train ticket.
2. **The signal is buried in noise.** The raw line spends tokens on the card
   mask, the duplicated date and the authorisation code, and a different code
   per purchase (`*WB1R672N5`, `*WB9Q111A2`) means Amazon never resembles
   itself, so history cannot attach there either.
3. **Destination matching misfires**, treating the placeholder as an existing
   account.

## Measurement

`tools/audit.py` (read-only, GET requests only) reports the share of withdrawals
per month whose payee is a placeholder, using the same list the classifier uses:

```bash
python3 tools/audit.py summary --months 6
```

On my instance, before the change: **50.7 % of September and 89.5 % of October
withdrawals carried a placeholder payee.** That is the bug in one number — for
the most recent month, nine withdrawals in ten were categorised with history
keyed on a single fake vendor.

## Proposed change

Group on a merchant parsed out of the description whenever the payee is not
usable, and send the model that merchant instead of the raw line.

- `internal/classifier/merchant.go` (new) — `ParseDescription()` applies
  ordered rules for the ways this bank writes card purchases, Bizum, transfers
  and bank fees, then normalises the result: legal-form suffixes (`SL`,
  `S.L.U.`), store numbers, trailing dates and reference numbers stripped,
  mojibake from older imports repaired, and an aggregator prefix dropped only
  when what follows is still recognisable. It also returns a payment method
  (`card`, `bizum`, `transfer`, ...), because a Bizum to a person and a card
  payment to a shop are rarely the same kind of expense.
- `Redact()` — the description as shown to the model, card mask and boilerplate
  removed.
- `classifier.GroupKey` — keeps a real `destination_name` when Firefly III has
  one, otherwise uses `MerchantInfo.Key()`; `blankNames` gains the placeholders
  seen in practice (`(unknown destination)`, `no registrado`, `pos purchase`,
  ...).
- `prompt.go` — shows `Merchant:`, `Payment method:` and a redacted
  description, and repeats the Firefly payee only when it differs.
- `pipeline.go` — ignores placeholder accounts, and rejects placeholder
  destinations when looking history up.

`ParseDescription` never fails: when nothing matches, the merchant is empty and
every call site falls back to current behaviour, so a bank with no rules behaves
exactly as it does today.

| Description | Merchant | Method |
|---|---|---|
| `COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : ...9408 EL 2026-10-12` | `AZAHAR NOCHE` | card |
| `COMPRA BIZUM VENTA BILLETES RENFE ... PEDIDO 000457060982` | `VENTA BILLETES RENFE` | bizum |
| `TRANSFERENCIA A FAVOR DE LA CUEVA DEL BARBERO CONCEPTO: FACTURA 12` | `LA CUEVA DEL BARBERO` | transfer |
| `COMPRA EN WWW.AMAZON*WB1R672N5, CON LA TARJETA : ...` | `AMAZON` | card |

## Validation

- `go vet ./...` and `go test ./...`; table tests for the parser, the grouping
  key and the prompt live beside the code (`merchant_test.go`,
  `groupkey_test.go`, `prompt_test.go`).
- **`DRY_RUN=true`** (new env var, default `false`) runs the whole decision path
  and logs each outcome without writing to Firefly III, so a change like this
  can be proven against real production data first.
- `tools/dryrun.py` finds the transactions that expose the bug, prints their
  state, and can serve an OpenAI-compatible stub endpoint that records every
  prompt it receives — so prompts can be inspected with no provider spend and
  no writes.
- After the dry run, every offending transaction was untouched in Firefly III
  and the prompts carried e.g. `Merchant: MUNDOLATINO SHELDON` with no card mask
  and no duplicated date.

## Compatibility

- No schema, config-format or API changes; a stored config from before the
  change loads unchanged.
- One new optional env var (`DRY_RUN`), and the new tools are opt-in scripts.
- Two unrelated bugs fixed while wiring this up, either of which may be worth
  taking on its own:
  1. an empty stored `base_url` overwrote `OPENAI_BASE_URL` from the environment
     on a fresh install, so a first boot could not be pointed at a local model;
  2. `.env` values kept their inline comments, so `TAG_PREFIX=ai  # comment`
     matched no tags.

## What I am asking for

Take the approach, the code, or neither — I have no stake beyond wanting it to
live somewhere maintained. I am not offering to maintain a fork of this project
long-term, so if these parsing rules land upstream they will get the care of a
real maintainer, and banks with other description formats can contribute their
rules to one place instead of to a scattering of forks.

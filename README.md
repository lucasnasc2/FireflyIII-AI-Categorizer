# Firefly III AI Categorizer

> [!WARNING]
> **This is a personal fork, published as-is and not maintained.** I built it to fix one specific problem in my own Firefly III data and I do not intend to keep it current. Dependencies age, and running unmaintained software is a security risk — the image tags here are frozen points in time, and `:latest` only ever means "latest from this fork". If you want software somebody is actually looking after, use [ejagombar/FireflyIII-AI-Categorizer](https://github.com/ejagombar/FireflyIII-AI-Categorizer). The most useful thing you can do with this repository is push [its one change](#why-this-fork-exists) upstream, where it can be maintained properly; the write-up is ready to submit in [UPSTREAM.md](UPSTREAM.md).

Automatically categorise transactions in [Firefly III](https://www.firefly-iii.org/) using an LLM — choose from OpenAI, Gemini, DeepSeek, or locally hosted. New withdrawals are classified into one of three outcomes and written back without any manual intervention. Process transactions automatically using a Firefly III webhook, or use the web interface to mass categorize transactions.

<img width="100%" alt="Jobs page" src="public/docs/images/jobs_page.webp" />

<p>
  <img width="49.5%" alt="Transactions page" src="public/docs/images/transactions_page.webp" />
  <img width="49.5%" alt="Settings page" src="public/docs/images/settings_page.webp" />
</p>

---

## Why this fork exists

My bank — Openbank, in Spain — never fills in Firefly III's `destination_name`. Every card purchase arrives like this, with the payee set to the literal placeholder `(unknown destination)`:

```
COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12
```

Upstream uses the payee as the key when it looks up similar past transactions to send the model as few-shot examples. With every withdrawal keyed on the same placeholder, those examples stop being "what we did last time at this shop" and become "what we did last time at *any* shop", so the category decided once for a bar comes back as precedent for a train ticket. The raw line also reaches the model verbatim, card mask, duplicated date and all.

Measured on my account before the fix: **50.7 % of September and 89.5 % of October withdrawals carried a placeholder payee.** In practice the categoriser was guessing, and the history made the guess look deliberate.

The cause was small: upstream's `isBlankName()` knows `(no name)`, `unknown` and `n/a`, but not `(unknown destination)`. That string was therefore accepted as a merchant name, history was grouped on it, and the category of one shop became the precedent for every other shop.

This fork makes the placeholder check generic, so no bank-specific rules are involved:

| | Upstream | This fork |
|---|---|---|
| Placeholder payees | treated as a merchant name | empty, generic names, `unknown…`, or anything Firefly III wrapped in parentheses are all recognised as "no merchant" |
| History key | the placeholder, for this bank | the transaction description (what upstream already does for nameless payees) |
| Prompt input | the raw line, printed twice, once as the "destination" | the raw line once, plus a note that the payee was not recorded and the merchant has to be read from the description |
| Destination matching | placeholder accounts offered as candidates and re-applied from history | placeholders filtered out, in both directions |
| Checking a build | not possible without writing to your books | `DRY_RUN=true` classifies, logs, writes nothing |
| Diagnostics | none | `tools/audit.py`, `tools/dryrun.py` |
| CI | publish only | `go vet` + `go test` gate the publish |

### Honest scope

**If your bank fills `destination_name` properly, upstream already does the right thing and this fork changes nothing for you.** If your bank does not, this fork stops the damage (unrelated merchants no longer share one history bucket) but does not invent a merchant name: the model works from the raw description, exactly as upstream intends for nameless payees.

The trade is calls, not accuracy. Keyed on the placeholder, upstream got no history at all for these transactions, so every one of them cost a model call. Keyed on the description, an exact repeat of the same description is now recognised — but banks that stamp a date or a reference into every line (mine does) produce a unique description each time, so by default most transactions still call the model.

That last gap is what [merchant learning](#learned-merchant-names-opt-in) closes: it is off by default, and when you turn it on the model names the merchant, the name is remembered, and repeat purchases at that shop are classified from history without a call.

Two things help you judge it on your own data, both read-only:

```bash
# Unknown-payee rate per month, AI tags, category spread
python3 tools/audit.py summary --months 6

# The individual descriptions, if you want to read them
python3 tools/audit.py descriptions --months 6 | head -40
```

## How this was built

Plainly: **with a lot of AI**, and I would rather say so than let you infer something else.

- Mine: the requirement, the investigation into what Openbank actually puts in `description` versus `destination_name`, acceptance testing against my own Firefly III instance, the decision to publish, and every push-back that shaped the code.
- The AI coding agent's ([Cline](https://cline.bot), running in VS Code): the classifier, prompt and pipeline changes, the tests, the audit and dry-run tools, the CI workflow and this documentation. It worked in a long loop in which I ran every step and checked the result.
- Nothing was taken on faith. `go vet` and `go test`, a local Docker build, then `DRY_RUN=true` against the live API with a stub provider so every prompt and every decision could be read while Firefly III stayed untouched, and only then a real run on freshly imported transactions.

Treat it accordingly: well tested against *my* data, lightly tested against anything else. It writes to your accounting records — read the diff first.

## Maintenance status

- **Not maintained.** No dependency work beyond the automation below, no support, and no promise that a Firefly III or provider API change will not break it.
- [Dependabot](.github/dependabot.yml) watches Go modules, GitHub Actions and the Docker base images, so stale dependencies at least become visible as pull requests. Merging one is a click; nothing obliges anybody to make it.
- This app has **no authentication** — see the warning at the top of Getting started. Do not expose it to the internet.
- AGPL-3.0, like upstream. The source for every image published here is this repository at the matching tag (`git checkout v1.0.0` for `:1.0.0`).
- Pin a release tag rather than following `:latest`: `latest` is only rebuilt when this fork's `main` moves, which it will not.

## Upstreaming

[UPSTREAM.md](UPSTREAM.md) writes the whole change up as an issue or pull-request body for [ejagombar/FireflyIII-AI-Categorizer](https://github.com/ejagombar/FireflyIII-AI-Categorizer): the problem, the measurement, the approach, the files touched, the validation and the compatibility notes. Copy it, from this account or any other, so the fix can live in a repository that somebody maintains — which is the only version of this that ages well.

---

## Features

The original project is [bahuma20/firefly-iii-ai-categorize](https://github.com/bahuma20/firefly-iii-ai-categorize), whose author archived the project. [openaccountants/firefly-iii-ai-categorize](https://github.com/openaccountants/firefly-iii-ai-categorize) forked it and introduced the three-outcome classification model. [ejagombar/FireflyIII-AI-Categorizer](https://github.com/ejagombar/FireflyIII-AI-Categorizer) forked that one and rewrote it from scratch with a new UI and additional features. [This fork](https://github.com/lucasnasc2/FireflyIII-AI-Categorizer) fixes [history grouped on placeholder payees](#why-this-fork-exists) on top of that and is otherwise the same:

- **Web UI.** A built-in dashboard, styled to match Firefly III's AdminLTE theme with light and dark mode support, provides real-time job monitoring, a transactions browser with manual re-categorisation, a category viewer, a review page for AI decisions, and a full settings page.
- **Multiple AI providers.** Supports OpenAI (and any OpenAI-compatible endpoint such as Ollama or Azure OpenAI), Google Gemini, and DeepSeek — switchable from the settings page.
- **Batch categorisation.** All uncategorised withdrawals can be classified in one operation from the UI or the command line.
- **Three-outcome classification model.** Carried forward from the [openaccountants fork](https://github.com/openaccountants/firefly-iii-ai-categorize). The AI discloses its confidence level (`CLASSIFIED`, `ASSUMED`, `NEEDS_REVIEW`) rather than silently guessing or doing nothing.
- **Transaction history context.** Past categorised transactions are sent to the model as few-shot examples, improving accuracy for recurring merchants. Repeated transactions from the same vendor are categorised automatically, skipping the AI calls entirely.
- **In-UI configuration.** All settings, including credentials, are saved through the web interface and persisted to disk across restarts. A custom context field lets you provide additional guidance to the AI (e.g., "I have no business expenses").
- **Destination account matching.** When enabled, the AI matches withdrawals to existing expense accounts or creates new ones automatically, with independent confidence levels for category and destination. The Review page provides a searchable dropdown to correct or create destination accounts manually. Transactions miscategorised as "Transfers" can be converted into real asset-account transfers.
- **Manual review workflow.** The Review tab groups unconfident classifications by merchant, with the AI's best guess pre-selected in a dropdown. Apply, skip, or correct per group — and when destination matching is on, set or create the payee alongside the category.
- **Transaction converter.** Withdrawals the AI flags as potential transfers between your own asset accounts appear in a "Convert to Transfers" panel on the Review page. Select a destination asset account and the categorizer deletes the withdrawal and creates a proper transfer — preserving the amount, date, and description.
- **In-app help page.** A built-in documentation tab walks through setup, features, and configuration with step-by-step instructions and screenshots, so you don't need to leave the app.
- **Rewritten in Go.** The Node.js/Express server has been replaced with a compiled Go binary, reducing resource usage and removing the Node.js runtime dependency.

---

## Getting started

> [!WARNING]
> This application has no built-in authentication — do not expose it directly to the internet. Run it on a private network or behind a reverse proxy with authentication (e.g. Nginx basic auth over HTTPS).

### 1. Run the server

The recommended way is Docker Compose with a volume mount so settings survive container restarts:

```yaml
services:
  categorizer:
    image: ghcr.io/lucasnasc2/fireflyiii-ai-categorizer:latest
    restart: unless-stopped
    ports:
      - "3000:3000"
    volumes:
      - ./categorizer-data:/data
```

```bash
docker compose up -d
```

Images are published to the [GitHub Container Registry](https://github.com/lucasnasc2/FireflyIII-AI-Categorizer/pkgs/container/fireflyiii-ai-categorizer) for `linux/amd64` and `linux/arm64`, tagged `latest`, the release version (`1.0.0`, `1.0`), and the commit (`sha-abcdef0`). Pin a release tag in the snippet above if you would rather not track `latest`.

Then open **http://localhost:3000** — you will be taken directly to the Settings page.

---

### 2. Connect to Firefly III

In the **Settings → Firefly III Connection** section, enter your Firefly III instance URL and a Personal Access Token.

To create a Personal Access Token in Firefly III, go to your profile and open the **OAuth** tab.

Scroll to **Personal Access Tokens** and click **Create new token**.

<img width="1383" height="567" alt="pat-creator" src="https://github.com/user-attachments/assets/85ada5ac-c56f-40cc-855b-8bc01192bc3c" />

Give it a name, confirm, and copy the token that appears — it is only shown once.


Paste the URL and token into the Settings page and click **Test connection** to verify, then **Save settings**.

---

### 3. Add an AI provider

In the **Settings → AI Provider** section, choose a provider and enter your API key.

**OpenAI** — create a key at [platform.openai.com/api-keys](https://platform.openai.com/api-keys). The default model is `gpt-4o-mini`, which is fast and inexpensive.

**Google Gemini** — go to [aistudio.google.com](https://aistudio.google.com), sign in, and click **Get API key**. The default model is `gemini-3.1-flash`.

**DeepSeek** — create an account at [platform.deepseek.com](https://platform.deepseek.com), go to **API keys**, and generate a new key. The default model is `deepseek-chat`.

**Local (Ollama)** — select OpenAI as the provider, set **Base URL** to your Ollama endpoint (e.g. `http://localhost:11434/v1`), leave the API key blank, and set the model name to whichever model you have pulled.

Click **Save settings** when done.

---

### 4. Set up the Firefly III webhook

In Firefly III, go to **Automation → Webhooks** and click **Create new webhook**.

<p>
  <img width="24%" alt="Webhooks navigation" src="https://github.com/user-attachments/assets/7829dffb-812e-4d93-a8ee-d79c45bf325c" />
  <img width="74%" alt="Webhook creation form" src="https://github.com/user-attachments/assets/20bea3bd-4dcb-4de6-a997-c44cf3692317" />
</p>

Set the URL to `http://<host>:3000/webhook` and save. New withdrawals will now be sent to the categorizer automatically.

<img width="100%" alt="Saved webhook" src="https://github.com/user-attachments/assets/de7a273e-9460-4b58-99ad-50cad3f09f21" />

> **Note:** Only withdrawal transactions without an existing category are processed. All other events are acknowledged and ignored.

The categorizer is running. New withdrawals added in Firefly III will appear in the **Jobs** tab as they are classified. Each job shows the outcome, the assigned category, and expandable detail with the AI's reasoning.

---

### Mass Categorization

To classify existing uncategorised transactions, use the **Batch Classification** panel on the Jobs tab — click **Check uncategorized** first to preview the count, then **Run batch** to process them.

You can also select individaul transactions or *all* transactions on the **Transactions** page and force them to be re-categorized.

<img width="100%" alt="Transactions page showing categorised results" src="public/docs/images/transactions_page.webp" />

The **Transactions** tab lets you browse withdrawals, filter by date, and manually trigger re-classification on any selection.

---

### Manual Review

The **Review** tab collects transactions the AI could not classify confidently — either flagged as **Needs Review** (no category set) or **AI Assumed** (a best guess that should be confirmed). Transactions are grouped by merchant, and for Assumed groups the AI's guess is pre-selected in the dropdown. Select a category for each group and click **Apply** to write them back to Firefly III. Use **Skip** to defer a group to the end of the queue without dismissing it.

<img width="1861" height="1199" alt="image" src="https://github.com/user-attachments/assets/4c7448f9-6d2e-403e-9ccc-eb5804faf75b" />

---

## How it works

```
POST /webhook  (Firefly III sends new transaction)
        │
        ▼
  Validate: withdrawal, no existing category
        │
        ▼
  Fetch history: recent categorised transactions (few-shot examples)
        │
        ▼
  Fetch categories from Firefly III
        │
        ▼
  (Optional) Fetch expense accounts (destination matching)
        │
        ▼
  Send to LLM with three-outcome prompt
        │
        ├── CLASSIFIED    → set category + tag "ai:classified"
        │                    └── destination → MATCH existing or CREATE new
        ├── ASSUMED       → set category + tag "ai:assumed" + assumption
        │                    └── destination → MATCH existing or CREATE new
        └── NEEDS_REVIEW  → tag "ai:needs-review", no category set
                             └── destination → may still be assigned
```

Results are written back to Firefly III with `fire_webhooks: false` to prevent
re-triggering. When destination matching is enabled, the flow also fetches
expense accounts and the LLM attempts to match or create a destination account
alongside the category. Destination confidence is independent of category
confidence — a transaction can be CLASSIFIED for its category but ASSUMED for
its destination, or vice versa.

Repeated transactions from the same merchant benefit from **history-based
matching**: when enough past transactions agree on the same category and
destination, the AI call is skipped entirely, reducing latency and API costs.

---

## CLI usage

### Build from source

Requires Go 1.22 or later.

```bash
git clone https://github.com/ejagombar/FireflyIII-AI-Categorizer.git
cd firefly-iii-ai-categorize
go build -o firefly-ai-categorize ./cmd/server
./firefly-ai-categorize
```

### Batch flag

The `--batch` flag classifies all uncategorised withdrawals and exits without starting the web server. This is useful for scripting or one-off runs:

```bash
./firefly-ai-categorize --batch
```

The server must already be configured — either through a pre-existing config file or via the environment variables below.

---

## Environment variables

All variables are optional. When they are absent, the server starts normally and credentials can be entered through the Settings page. Environment variables take precedence over built-in defaults, but values saved through the UI take precedence over environment variables.

### Firefly III connection

| Variable | Default | Description |
|----------|---------|-------------|
| `FIREFLY_URL` | — | URL of your Firefly III instance |
| `FIREFLY_PERSONAL_TOKEN` | — | Firefly III Personal Access Token |

### AI provider

| Variable | Default | Description |
|----------|---------|-------------|
| `AI_PROVIDER` | `openai` | Provider: `openai`, `gemini`, or `deepseek` |
| `OPENAI_API_KEY` | — | OpenAI API key |
| `OPENAI_MODEL` | `gpt-4o-mini` | OpenAI model name |
| `OPENAI_BASE_URL` | — | Override for OpenAI-compatible APIs (Ollama, Azure, etc.) |
| `GEMINI_API_KEY` | — | Google Gemini API key |
| `GEMINI_MODEL` | `gemini-3.1-flash-lite` | Gemini model name. That is the binary's built-in default; leave it unset unless your account needs a different model. |
| `DEEPSEEK_API_KEY` | — | DeepSeek API key |
| `DEEPSEEK_MODEL` | `deepseek-chat` | DeepSeek model name |

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3000` | HTTP port to listen on |
| `ENABLE_UI` | `true` | Set to `false` to disable the web UI |
| `CONFIG_FILE` | `/data/config.json` (Docker) | Path to the persistent settings file |
| `TAG_PREFIX` | `ai` | Prefix for tags written to transactions (`ai:classified`, etc.) |

### Processing

| Variable | Default | Description |
|----------|---------|-------------|
| `WORKER_CONCURRENCY` | `1` | Parallel webhook jobs (increase with caution) |
| `BATCH_CONCURRENCY` | `5` | Parallel jobs during batch runs |
| `HISTORY_CONTEXT_LIMIT` | `5` | Past transactions sent to the model as context examples |
| `HISTORY_LOOKBACK_DAYS` | `90` | How far back to search for history examples |
| `HISTORY_CACHE_TTL` | `10m` | How long to cache the history lookup |
| `DRY_RUN` | `false` | Classify as usual but never write to Firefly III — every decision is logged instead. Use it to validate a new build against production data before letting it write. |
| `DESTINATION_MATCH_ENABLED` | `false` | Env-var equivalent of Settings → Behaviour → "Set destination account automatically". |
| `MERCHANT_LOOKUP_ENABLED` | `false` | Env-var equivalent of Settings → Behaviour → "Learn merchant names from descriptions". Asks the model to name the merchant and remembers it so repeat descriptions skip the call. In-memory, never written to Firefly III. |

---

## Payees that are not merchants

Banks rarely fill in a usable payee. A Spanish card purchase looks like this:

```
COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12
```

Firefly III stores that whole line as the description and leaves `destination_name` empty or set to a placeholder such as `(unknown destination)`. Grouping history on that placeholder makes every unrelated merchant look like the same vendor, so the placeholder is first recognised as "no merchant":

- empty, or one of a short list of generic names (`-`, `n/a`, `no name`, `unknown`, `POS Purchase`, `card purchase`, `purchase`, `transfer`, `no registrado`, …),
- anything Firefly III wrapped in parentheses — `(unknown destination)`, `(no payee)`, `(sin nombre)` — which is the convention it uses for placeholder accounts,
- anything mentioning `unknown` or `no name`.

When a payee is blank, history is keyed on the description (upstream's existing fallback), placeholder accounts are never offered to the model as destination candidates, a destination matched from history is dropped if it points at a placeholder, and the prompt says so plainly:

```
Transaction to classify:
  Payee: not recorded by the bank — identify the merchant from the description
  Description: COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12
  Amount: 42.10
```

A real `destination_name` is never second-guessed: when Firefly III holds an actual merchant, that name keeps winning.

`internal/classifier/classifier_test.go` covers the name check, the regression (two merchants behind one placeholder keep distinct history keys) and the prompt shape; `internal/cache/cache_test.go` covers the learned-merchant keys. Both use only the standard library.

```bash
go test ./internal/classifier ./internal/cache
```

### Learned merchant names (opt-in)

`MERCHANT_LOOKUP_ENABLED=true`, or *Settings → Behaviour → "Learn merchant names from descriptions"*.

Left at the default, a transaction whose description is new always costs a model call, because the description is the history key and the bank changes it every time. With this setting on, the classification call is also asked to name the merchant — one extra string in a response that was being generated anyway, not a second request. The name is then remembered against a fingerprint of the description: letters only, upper-cased, so the card mask, amount, date and order number drop out. Two purchases at the same shop produce one fingerprint.

From then on:

- history lookup uses the learned merchant as the key, so transactions from that shop group together even though no two descriptions are identical;
- when the history match is confident enough, the model call is skipped entirely — the same short-circuit that exists for named payees;
- Firefly III is never written to with the merchant name. It stays a lookup key.

What it costs and what it does not do:

- One model call per merchant, the first time it is seen. Nothing can infer a merchant without looking at it once.
- The map lives in memory and is lost on restart: the first transaction per merchant after a restart re-learns it.
- The learned name is the model's, so it can vary between providers and models. That is why this is a setting rather than the default — the fallback (key on the description) never invents anything.
- The fingerprint only separates merchants as well as the description does. If your bank writes identical descriptions for two different shops, they will share a key; nothing here can tell them apart.

### Validating a build with `DRY_RUN`

`DRY_RUN=true` runs the full decision path and logs every outcome without touching Firefly III. Pair it with `tools/dryrun.py`, which finds the transactions most likely to expose a payee-grouping bug and can stand in for the AI provider:

```bash
# 1. Stub OpenAI-compatible endpoint: answers with a fixed category and
#    records every prompt it receives (default /tmp/stub-prompts.log).
#    Add --merchant to also test merchant learning.
python3 tools/dryrun.py stub --merchant 'Some Merchant' &

# 2. Start the app against the stub, with writes disabled. Under Docker the
#    stub's loopback address is not the container's, so use network host.
docker run --rm --network host --env-file .env \
  -e DRY_RUN=true -e MERCHANT_LOOKUP_ENABLED=true -e PORT=13099 \
  -e OPENAI_BASE_URL=http://127.0.0.1:18080/v1 -e OPENAI_API_KEY=stub \
  firefly-aicat-categorizer:dev

# 3. Journal ids whose payee is still a placeholder — these are the ones that
#    used to be grouped together under "(unknown destination)"
python3 tools/dryrun.py ids

# 4. Classify them, then confirm nothing changed in Firefly III. Run it twice:
#    the second run should show history-matched / skipped model calls.
curl -X POST localhost:13099/batch/run -H 'Content-Type: application/json' \
  -d '{"filter":{"transaction_ids":["528","527"]},"force":true}'
python3 tools/dryrun.py show 528
```

Read `/tmp/stub-prompts.log` to see the exact prompt the model would have received; `docker compose logs` shows the `dry-run: skipping transaction update` lines with the decision that would have been written.

---

## Privacy

Transaction descriptions, destination names, and amounts are sent to the configured AI provider. To keep data on-premises, set `OPENAI_BASE_URL` to a locally-hosted model via [Ollama](https://ollama.com/).

---

## License

AGPL-3.0 — same as the original project.

## Destination account matching

When you enable **Set destination account automatically** in Settings → Behaviour, the AI will also attempt to match or create the expense account (payee) for each withdrawal:

- **MATCH** — the AI finds an existing expense account that corresponds to the merchant.
- **CREATE** — the AI is confident a new account is needed and creates it in Firefly III automatically.

This requires an extra API call to fetch your expense accounts, modestly increasing latency and AI token usage. When disabled, the AI only determines the category and the expense account is left untouched.

On the **Review** page, when enabled, each group card gains a searchable Destination Account field so you can correct or set the payee manually during review. Typing a name that doesn't match any existing account shows a **NEW** badge, indicating a new expense account will be created when you apply.

Additionally, transactions that the AI categorised as "Transfers" appear in a **Convert to Transfers** box below the review cards. These are withdrawals that should actually be transfers between your own asset accounts. Select a destination asset account, and the categorizer deletes the withdrawal and creates a proper transfer in its place.

## Credits

- Original project by [bahuma20](https://github.com/bahuma20/firefly-iii-ai-categorize)
- Three-outcome classification model by [openaccountants/firefly-iii-ai-categorize](https://github.com/openaccountants/firefly-iii-ai-categorize), the intermediate fork this project is based on

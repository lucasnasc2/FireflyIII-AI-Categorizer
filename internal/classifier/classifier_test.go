package classifier

import (
	"strings"
	"testing"
)

const payeeForPrompt = "COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12"

// A payee that carries no merchant information must be recognised as blank.
// "(unknown destination)" is what Firefly III fills in when the bank supplies no
// payee, and it was missing from the original list.
func TestIsBlankName(t *testing.T) {
	blank := []string{
		"",
		"   ",
		"-",
		"n/a",
		"n.a.",
		"no name",
		"(no name)",
		"unknown",
		"Unknown Destination",
		"(unknown destination)",
		"(Unknown destination #12)",
		"Unknown payee",
		"no registrado",
		"POS Purchase",
		"(transfer)",
		"(sin nombre)", // unknown locale: the parentheses convention covers it
	}
	for _, name := range blank {
		if !IsBlankName(name) {
			t.Errorf("IsBlankName(%q) = false, want true", name)
		}
	}

	present := []string{
		"Amazon",
		"Mercadona",
		"Café Central",
		"AZAHAR NOCHE, SL",
		"TransferWise",
		"Delta Airlines",
	}
	for _, name := range present {
		if IsBlankName(name) {
			t.Errorf("IsBlankName(%q) = true, want false", name)
		}
	}
}

// The regression: two different merchants sharing the same placeholder payee must
// not share a history bucket.
func TestGroupKeyPlaceholderDoesNotCollapseMerchants(t *testing.T) {
	const payee = "(unknown destination)"
	azahar := "COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12"
	renfe := "COMPRA BIZUM VENTA BILLETES RENFE 2026-08-25 PEDIDO 000457060982"

	gotA := GroupKey(payee, azahar)
	gotB := GroupKey(payee, renfe)

	if gotA == gotB {
		t.Fatalf("GroupKey collapsed two merchants onto %q", gotA)
	}
	if gotA != azahar || gotB != renfe {
		t.Errorf("GroupKey should fall back to the description, got %q and %q", gotA, gotB)
	}
}

// A real payee keeps winning over the description.
func TestGroupKeyKeepsRealPayee(t *testing.T) {
	if got := GroupKey("Mercadona", "COMPRA TARJETA 2026-10-12"); got != "Mercadona" {
		t.Errorf("GroupKey() = %q, want %q", got, "Mercadona")
	}
}

// The prompt must not present a placeholder as if it were the merchant, and it
// must tell the model where to look instead.
func TestBuildUserPromptPlaceholderPayee(t *testing.T) {
	desc := "COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12"
	prompt := buildUserPrompt(Request{
		DestinationName: "(unknown destination)",
		Description:     desc,
	})

	if strings.Contains(prompt, "(unknown destination)") {
		t.Errorf("prompt repeats the placeholder payee:\n%s", prompt)
	}
	if !strings.Contains(prompt, "not recorded by the bank") {
		t.Errorf("prompt does not flag the missing payee:\n%s", prompt)
	}
	if strings.Count(prompt, desc) != 1 {
		t.Errorf("prompt should show the description exactly once:\n%s", prompt)
	}
}

// A real payee is still shown as the payee.
func TestBuildUserPromptRealPayee(t *testing.T) {
	prompt := buildUserPrompt(Request{
		DestinationName: "Mercadona",
		Description:     "COMPRA TARJETA 2026-10-12",
	})

	if !strings.Contains(prompt, "Payee: Mercadona") {
		t.Errorf("prompt lost the real payee:\n%s", prompt)
	}
	if strings.Contains(prompt, "not recorded") {
		t.Errorf("prompt claims the payee is missing:\n%s", prompt)
	}
}

// Placeholder accounts are filtered out of the candidate list offered to the model.
func TestBuildUserPromptSkipsPlaceholderAccounts(t *testing.T) {
	prompt := buildUserPrompt(Request{
		DestinationName: "Mercadona",
		Description:     "COMPRA TARJETA 2026-10-12",
		ExpenseAccounts: []AccountCandidate{
			{ID: "1", Name: "(unknown destination)"},
			{ID: "2", Name: "Mercadona"},
		},
	})

	if strings.Contains(prompt, "(unknown destination)") {
		t.Errorf("prompt offers a placeholder account as a candidate:\n%s", prompt)
	}
	if !strings.Contains(prompt, "  - Mercadona") {
		t.Errorf("prompt dropped the real account:\n%s", prompt)
	}
}

// The history block must be labelled with the key the history was actually looked
// up by, otherwise a learned merchant is invisible in the prompt.
func TestBuildUserPromptUsesHistoryKey(t *testing.T) {
	req := Request{
		DestinationName: "(unknown destination)",
		Description:     payeeForPrompt,
		HistoryKey:      "AZAHAR NOCHE",
		History: []HistoricalEntry{
			{Description: "COMPRA EN AZAHAR NOCHE 2026-10-01", CategoryName: "Eating Out"},
		},
	}
	prompt := buildUserPrompt(req)
	if !strings.Contains(prompt, `Past classifications for "AZAHAR NOCHE"`) {
		t.Errorf("history block is not labelled with the lookup key:\n%s", prompt)
	}

	// Without a key, the description is still used as the label.
	req.HistoryKey = ""
	if prompt := buildUserPrompt(req); !strings.Contains(prompt, `Past classifications for "`+payeeForPrompt+`"`) {
		t.Errorf("fallback label missing:\n%s", prompt)
	}
}

// merchant learned the first time, even though the date and card mask differ.
func TestDescriptionFingerprint(t *testing.T) {
	a := DescriptionFingerprint("COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12")
	b := DescriptionFingerprint("COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX1111 EL 2026-11-03")
	if a != b {
		t.Errorf("same merchant should share a fingerprint:\n a=%q\n b=%q", a, b)
	}

	c := DescriptionFingerprint("COMPRA BIZUM VENTA BILLETES RENFE 2026-08-25 PEDIDO 000457060982")
	if a == c {
		t.Errorf("different merchants must not share a fingerprint (%q)", a)
	}
	if strings.ContainsAny(a, "0123456789:,-*.") {
		t.Errorf("fingerprint %q should keep letters only", a)
	}
	if DescriptionFingerprint("") != "" {
		t.Error("empty description should yield an empty fingerprint")
	}
}

// The merchant is only accepted from the model when it was asked for.
func TestParseResponseMerchant(t *testing.T) {
	raw := `{"outcome":"CLASSIFIED","category":{"name":"Groceries","confidence":"CLASSIFIED"},` +
		`"reason":"supermarket","assumption":null,"merchant":"  Mercadona "}`
	cats := []Category{{Name: "Groceries"}}

	got := parseResponse(raw, "prompt", Request{Categories: cats, MerchantLookup: true})
	if got.Merchant != "Mercadona" {
		t.Errorf("Merchant = %q, want %q", got.Merchant, "Mercadona")
	}

	// Without the opt-in, the same response must not produce a merchant.
	if got := parseResponse(raw, "prompt", Request{Categories: cats}); got.Merchant != "" {
		t.Errorf("Merchant = %q, want empty when lookup is off", got.Merchant)
	}

	// Placeholders are rejected so they can never become a history key.
	placeholder := `{"outcome":"CLASSIFIED","category":{"name":"Groceries","confidence":"CLASSIFIED"},` +
		`"reason":"unknown","assumption":null,"merchant":"(unknown destination)"}`
	if got := parseResponse(placeholder, "prompt", Request{Categories: cats, MerchantLookup: true}); got.Merchant != "" {
		t.Errorf("Merchant = %q, want empty for a placeholder", got.Merchant)
	}
}

// The system prompt gains the merchant instructions only when asked.
func TestBuildSystemPromptMerchantLookup(t *testing.T) {
	off := BuildSystemPrompt("", false, false)
	if strings.Contains(off, `"merchant"`) {
		t.Errorf("merchant schema leaked into a prompt that did not ask for it:\n%s", off)
	}

	on := BuildSystemPrompt("", false, true)
	if !strings.Contains(on, `"merchant"`) || !strings.Contains(on, "Merchant rules:") {
		t.Errorf("merchant instructions missing:\n%s", on)
	}

	// Custom context still comes last, after the merchant block.
	withCtx := BuildSystemPrompt("I shop at Mercadona", false, true)
	if !strings.HasSuffix(strings.TrimSpace(withCtx), "I shop at Mercadona") {
		t.Errorf("custom context is no longer last:\n%s", withCtx)
	}
}

package classifier

import (
	"strings"
	"testing"
)

func TestBuildUserPromptUsesParsedMerchant(t *testing.T) {
	amount := 12.35
	req := Request{
		Categories:      []Category{{Name: "Groceries"}, {Name: "Other"}},
		DestinationName: "(unknown destination)",
		Description:     "COMPRA EN MERCADONA, CON LA TARJETA :5412XXXXXXXX1234 FECHA 2025-10-06",
		Amount:          &amount,
		ExpenseAccounts: []AccountCandidate{
			{ID: "7", Name: "(unknown destination)"},
			{ID: "9", Name: "Mercadona"},
		},
		History: []HistoricalEntry{
			{Description: "COMPRA EN MERCADONA, CON LA TARJETA :5412XXXXXXXX1234", CategoryName: "Groceries"},
		},
	}

	got := buildUserPrompt(req)

	if !strings.Contains(got, "Merchant: MERCADONA") {
		t.Errorf("prompt does not surface the parsed merchant:\n%s", got)
	}
	if !strings.Contains(got, "Payment method: card purchase") {
		t.Errorf("prompt does not surface the payment method:\n%s", got)
	}
	if !strings.Contains(got, "  - Mercadona") {
		t.Errorf("prompt does not list the real expense account:\n%s", got)
	}
	if strings.Contains(got, "unknown destination") {
		t.Errorf("prompt leaks the placeholder payee to the model:\n%s", got)
	}
	if strings.Contains(strings.ToUpper(got), "XXXX") {
		t.Errorf("prompt leaks the card mask:\n%s", got)
	}
	if strings.Contains(got, "2025-10-06") {
		t.Errorf("prompt leaks the duplicated date:\n%s", got)
	}
}

// TestBuildUserPromptWithoutAccounts makes sure the "Existing expense accounts"
// header is not emitted when every account is a placeholder.
func TestBuildUserPromptWithoutAccounts(t *testing.T) {
	req := Request{
		Categories:      []Category{{Name: "Groceries"}},
		Description:     "COMPRA EN MERCADONA",
		ExpenseAccounts: []AccountCandidate{{ID: "7", Name: "(unknown destination)"}},
	}

	got := buildUserPrompt(req)
	if strings.Contains(got, "Existing expense accounts") {
		t.Errorf("prompt lists an empty expense-account section:\n%s", got)
	}
}

// TestBuildUserPromptMerchantOverride verifies that a caller-supplied merchant
// (e.g. from the Review page) is not overwritten by the parser.
func TestBuildUserPromptMerchantOverride(t *testing.T) {
	req := Request{
		Categories:  []Category{{Name: "Groceries"}},
		Description: "COMPRA EN MERCADONA, CON LA TARJETA :5412XXXXXXXX1234",
		Merchant:    "Mercadona S.A.",
	}

	got := buildUserPrompt(req)
	if !strings.Contains(got, "Merchant: Mercadona S.A.") {
		t.Errorf("caller-supplied merchant was not honoured:\n%s", got)
	}
}

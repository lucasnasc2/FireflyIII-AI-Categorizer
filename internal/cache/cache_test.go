package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/openaccountants/firefly-iii-ai-categorize/internal/classifier"
)

const (
	payeeOne = "COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12"
	payeeTwo = "COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX1111 EL 2026-11-03"
)

// Without a learned merchant, a placeholder payee falls back to the description,
// which is what the classifier would use on its own.
func TestKeyFallsBackToDescription(t *testing.T) {
	c := New(nil, 0, 0)
	if got, want := c.Key("(unknown destination)", payeeOne), classifier.GroupKey("(unknown destination)", payeeOne); got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}

// Once a merchant is learned for a description, every transaction with the same
// description groups under it - including one with a different date and card
// mask, which is the case this setting exists for.
func TestKeyUsesLearnedMerchant(t *testing.T) {
	c := New(nil, 0, 0)
	c.SetMerchant(payeeOne, "AZAHAR NOCHE")

	if got := c.Key("(unknown destination)", payeeOne); got != "AZAHAR NOCHE" {
		t.Errorf("Key() = %q, want %q", got, "AZAHAR NOCHE")
	}
	if got := c.Key("(unknown destination)", payeeTwo); got != "AZAHAR NOCHE" {
		t.Errorf("a purchase on another day should reuse the merchant, got %q", got)
	}
}

// A real payee always wins over anything learned.
func TestKeyPrefersRealPayee(t *testing.T) {
	c := New(nil, 0, 0)
	c.SetMerchant(payeeOne, "AZAHAR NOCHE")
	if got := c.Key("Mercadona", payeeOne); got != "Mercadona" {
		t.Errorf("Key() = %q, want the real payee %q", got, "Mercadona")
	}
}

// Learning is per description: an unrelated bank line must not inherit it.
func TestSetMerchantIsScoped(t *testing.T) {
	c := New(nil, 0, 0)
	c.SetMerchant(payeeOne, "AZAHAR NOCHE")

	other := "COMPRA BIZUM VENTA BILLETES RENFE 2026-08-25 PEDIDO 000457060982"
	if got := c.Merchant(other); got != "" {
		t.Errorf("Merchant(%q) = %q, want empty", other, got)
	}
	if got := c.Key("(unknown destination)", other); strings.Contains(got, "AZAHAR") {
		t.Errorf("unrelated description inherited a merchant: %q", got)
	}
}

// This is the payoff: a purchase at a shop learned earlier is found from history
// even though the bank wrote a different description for it. The pipeline uses
// exactly this lookup to skip the model call.
func TestGetHistoryFollowsLearnedMerchant(t *testing.T) {
	c := New(nil, time.Minute, 30)
	c.fetchedAt = time.Now() // pretend the refresh already happened

	// A purchase from before anything was learned: keyed on its description.
	c.Append(classifier.HistoricalEntry{
		Description:  payeeOne,
		CategoryName: "Restaurants",
		GroupKey:     classifier.GroupKey("(unknown destination)", payeeOne),
	})

	// Another purchase at the same shop, a different day and card mask.
	if got := c.GetHistory(context.Background(), c.Key("(unknown destination)", payeeTwo), 5); len(got) != 0 {
		t.Fatalf("before learning, history should not match, got %d entries", len(got))
	}

	c.SetMerchant(payeeTwo, "AZAHAR NOCHE")

	got := c.GetHistory(context.Background(), c.Key("(unknown destination)", payeeTwo), 5)
	if len(got) != 1 {
		t.Fatalf("after learning, got %d entries, want 1", len(got))
	}
	if got[0].CategoryName != "Restaurants" {
		t.Errorf("CategoryName = %q, want %q", got[0].CategoryName, "Restaurants")
	}
}

func TestSetMerchantIgnoresEmpty(t *testing.T) {
	c := New(nil, 0, 0)
	c.SetMerchant("", "Mercadona")
	c.SetMerchant("1234", "Mercadona") // no letters -> empty fingerprint
	if got := c.Merchant(""); got != "" {
		t.Errorf("Merchant(\"\") = %q, want empty", got)
	}
}

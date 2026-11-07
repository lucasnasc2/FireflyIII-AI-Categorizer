package classifier

import (
	"testing"
)

func TestGroupKey(t *testing.T) {
	const (
		mercadona = "COMPRA EN MERCADONA, CON LA TARJETA :5412XXXXXXXX1234"
		estanco   = "COMPRA EN ESTANCO, CON LA TARJETA :5412XXXXXXXX1234"
	)

	tests := []struct {
		name        string
		destination string
		description string
		want        string
	}{
		{
			name:        "real destination wins over the parsed merchant",
			destination: "Mercadona",
			description: mercadona,
			want:        "Mercadona",
		},
		{
			name:        "placeholder destination falls through to parsed merchant",
			destination: "(unknown destination)",
			description: mercadona,
			want:        "MERCADONA",
		},
		{
			name:        "no-name placeholder falls through to parsed merchant",
			destination: "(no name)",
			description: "COMPRA EN AZAHAR NOCHE, SL CON LA TARJETA :5412XXXXXXXX1234",
			want:        "AZAHAR NOCHE",
		},
		{
			name:        "already clean description is its own key",
			destination: "",
			description: "LA CUEVA DEL BARBERO",
			want:        "LA CUEVA DEL BARBERO",
		},
		{
			name:        "nothing available",
			destination: "",
			description: "",
			want:        "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := GroupKey(tc.destination, tc.description); got != tc.want {
				t.Errorf("GroupKey(%q, %q) = %q, want %q", tc.destination, tc.description, got, tc.want)
			}
		})
	}

	// Regression: every unnamed withdrawal used to share the single key
	// "(unknown destination)", which fed unrelated merchants into the model as
	// if they were one vendor. Distinct merchants must now produce distinct keys.
	if GroupKey("(unknown destination)", mercadona) == GroupKey("(unknown destination)", estanco) {
		t.Error("distinct merchants share one group key; the placeholder is leaking through")
	}
}

// TestGroupKeyCollapsesAuthCodes is the mirror image of the collision bug:
// Amazon appends a different authorisation code to every purchase, so one
// merchant used to splinter into one bucket per order.
func TestGroupKeyCollapsesAuthCodes(t *testing.T) {
	first := "COMPRA EN WWW.AMAZON*WB1R672N5, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-10"
	second := "COMPRA EN WWW.AMAZON*0I7MD0745, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-11-02"

	firstKey := GroupKey("(unknown destination)", first)
	secondKey := GroupKey("(unknown destination)", second)

	if firstKey != secondKey {
		t.Errorf("one merchant split into two buckets: %q != %q", firstKey, secondKey)
	}
	if firstKey != "AMAZON" {
		t.Errorf("GroupKey = %q, want %q", firstKey, "AMAZON")
	}
}

func TestIsBlankName(t *testing.T) {
	blank := []string{
		"", "  ", "no name", "(no name)", "unknown", "n/a",
		"(unknown destination)", "unknown destination", "Unknown Destination",
		"(unknown destination) #12",
		"pos purchase", "Transfer", "No Registrado",
	}
	for _, s := range blank {
		if !isBlankName(s) {
			t.Errorf("isBlankName(%q) = false, want true", s)
		}
		if IsUsableAccountName(s) {
			t.Errorf("IsUsableAccountName(%q) = true, want false", s)
		}
	}

	real := []string{"Mercadona", "LA CUEVA DEL BARBERO", "ALBERTO PIÑEIRO PRADO"}
	for _, s := range real {
		if isBlankName(s) {
			t.Errorf("isBlankName(%q) = true, want false", s)
		}
		if !IsUsableAccountName(s) {
			t.Errorf("IsUsableAccountName(%q) = false, want true", s)
		}
	}
}

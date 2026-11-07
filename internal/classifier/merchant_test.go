package classifier

import (
	"strings"
	"testing"
)

// These are verbatim descriptions taken from a real Openbank (Spain) export.
// destination_name is either empty or "(unknown destination)" for every one of
// them, which is exactly why the merchant has to be parsed out of the text.
func TestParseDescription(t *testing.T) {
	tests := []struct {
		name         string
		description  string
		wantMerchant string
		wantMethod   PaymentMethod
	}{
		{
			name:         "card purchase with legal suffix",
			description:  "COMPRA EN AZAHAR NOCHE, SL CON LA TARJETA :5412XXXXXXXX1234",
			wantMerchant: "AZAHAR NOCHE",
			wantMethod:   MethodCard,
		},
		{
			name:         "card purchase with trailing date",
			description:  "COMPRA EN MERCADONA, CON LA TARJETA :5412XXXXXXXX1234 FECHA 2025-10-06",
			wantMerchant: "MERCADONA",
			wantMethod:   MethodCard,
		},
		{
			name:         "card purchase without card suffix",
			description:  "COMPRA EN SUPERMERCADO CHINO",
			wantMerchant: "SUPERMERCADO CHINO",
			wantMethod:   MethodCard,
		},
		{
			name:         "bizum used to pay a merchant",
			description:  "COMPRA BIZUM VENTA BILLETES RENFE",
			wantMerchant: "VENTA BILLETES RENFE",
			wantMethod:   MethodBizum,
		},
		{
			name:         "bizum to a person with broken enye",
			description:  "BIZUM A FAVOR DE ALBERTO PI¥EIRO PRADO CONCEPTO: CENA",
			wantMerchant: "ALBERTO PIÑEIRO PRADO",
			wantMethod:   MethodBizum,
		},
		{
			name:         "transfer with concepto reference",
			description:  "TRANSFERENCIA A FAVOR DE LA CUEVA DEL BARBERO CONCEPTO: FACTURA 12",
			wantMerchant: "LA CUEVA DEL BARBERO",
			wantMethod:   MethodTransfer,
		},
		{
			name:         "instant transfer without reference",
			description:  "TRANSFERENCIA INMEDIATA A FAVOR DE JUAN PEREZ",
			wantMerchant: "JUAN PEREZ",
			wantMethod:   MethodTransfer,
		},
		{
			name:         "bank fee keeps the whole description",
			description:  "COMISION MANTENIMIENTO CUENTA",
			wantMerchant: "COMISION MANTENIMIENTO CUENTA",
			wantMethod:   MethodFee,
		},
		{
			name:         "direct debit with legal suffix",
			description:  "RECIBO IBERDROLA CLIENTES SAU",
			wantMerchant: "IBERDROLA CLIENTES",
			wantMethod:   MethodDirect,
		},
		{
			name:         "recurring manual payment",
			description:  "PAGO DE ALQUILER",
			wantMerchant: "ALQUILER",
			wantMethod:   MethodUnknown,
		},
		{
			name:         "already clean description",
			description:  "LA CUEVA DEL BARBERO",
			wantMerchant: "LA CUEVA DEL BARBERO",
			wantMethod:   MethodUnknown,
		},
		{
			name:         "already clean description two words",
			description:  "TAXI RIDE",
			wantMerchant: "TAXI RIDE",
			wantMethod:   MethodUnknown,
		},
		{
			name:         "verbatim openbank card format",
			description:  "COMPRA EN MUNDOLATINO SHELDON, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-10",
			wantMerchant: "MUNDOLATINO SHELDON",
			wantMethod:   MethodCard,
		},
		{
			name:         "legal form followed by a second comma",
			description:  "COMPRA EN AZAHAR NOCHE, SL, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-12",
			wantMerchant: "AZAHAR NOCHE",
			wantMethod:   MethodCard,
		},
		{
			name:         "wallet aggregator prefix",
			description:  "Wallet openbank COMPRA EN MERCADONA, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-07-14",
			wantMerchant: "MERCADONA",
			wantMethod:   MethodCard,
		},
		{
			name:         "bizum with order reference and date",
			description:  "COMPRA BIZUM VENTA BILLETES RENFE 2026-08-25 PEDIDO 000457060982",
			wantMerchant: "VENTA BILLETES RENFE",
			wantMethod:   MethodBizum,
		},
		{
			name:         "transfer concept without colon and dotted legal form",
			description:  "TRANSFERENCIA INMEDIATA A FAVOR DE TARTASYNATAS S.L. CONCEPTO Lucas Nascimento",
			wantMerchant: "TARTASYNATAS",
			wantMethod:   MethodTransfer,
		},
		{
			name:         "card aggregator authorisation code",
			description:  "COMPRA EN WWW.AMAZON*WB1R672N5, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-10",
			wantMerchant: "AMAZON",
			wantMethod:   MethodCard,
		},
		{
			name:         "authorisation code with a space after the star",
			description:  "COMPRA EN AIRBNB * HMZT2MWR5C, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-07-15",
			wantMerchant: "AIRBNB",
			wantMethod:   MethodCard,
		},
		{
			name:         "product name after the star is kept",
			description:  "COMPRA EN GOOGLE*YOUTUBEPREMIUM, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-10",
			wantMerchant: "GOOGLE*YOUTUBEPREMIUM",
			wantMethod:   MethodCard,
		},
		{
			name:         "store number is dropped",
			description:  "COMPRA EN DIA 22039, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-10",
			wantMerchant: "DIA",
			wantMethod:   MethodCard,
		},
		{
			name:         "domain suffix is dropped",
			description:  "COMPRA EN ALIEXPRESS.COM, CON LA TARJETA : XXXXXXXXXXXX9408 EL 2026-10-10",
			wantMerchant: "ALIEXPRESS",
			wantMethod:   MethodCard,
		},
		{
			name:         "empty description",
			description:  "",
			wantMerchant: "",
			wantMethod:   MethodUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseDescription(tc.description)
			if got.Merchant != tc.wantMerchant {
				t.Errorf("Merchant = %q, want %q", got.Merchant, tc.wantMerchant)
			}
			if got.Method != tc.wantMethod {
				t.Errorf("Method = %q, want %q", got.Method, tc.wantMethod)
			}
		})
	}
}

// TestParseDescriptionCoverage asserts that the placeholder boilerplate never
// survives into the merchant field: no card mask, no "CON LA TARJETA", no date.
func TestParseDescriptionCoverage(t *testing.T) {
	descriptions := []string{
		"COMPRA EN AZAHAR NOCHE, SL CON LA TARJETA :5412XXXXXXXX1234",
		"COMPRA EN MERCADONA, CON LA TARJETA :5412XXXXXXXX1234 FECHA 2025-10-06",
		"COMPRA EN SUPERMERCADO CHINO",
		"COMPRA BIZUM VENTA BILLETES RENFE",
	}
	junk := []string{"TARJETA", "XXXX", "FECHA", ":54"}
	for _, d := range descriptions {
		got := ParseDescription(d).Merchant
		if got == "" {
			t.Errorf("ParseDescription(%q).Merchant is empty, want a merchant", d)
		}
		upper := strings.ToUpper(got)
		for _, j := range junk {
			if strings.Contains(upper, j) {
				t.Errorf("ParseDescription(%q).Merchant = %q still contains %q", d, got, j)
			}
		}
	}
}

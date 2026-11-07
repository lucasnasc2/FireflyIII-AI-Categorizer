package classifier

import (
	"regexp"
	"strings"
	"unicode"
)

// PaymentMethod describes how the money left the account, as reported by the
// bank inside the description. It matters for classification because a Bizum,
// a bank transfer and a card purchase are rarely the same kind of expense.
type PaymentMethod string

const (
	MethodCard     PaymentMethod = "card purchase"
	MethodBizum    PaymentMethod = "Bizum (instant mobile payment)"
	MethodTransfer PaymentMethod = "bank transfer"
	MethodFee      PaymentMethod = "bank fee"
	MethodDirect   PaymentMethod = "direct debit"
	MethodCash     PaymentMethod = "cash withdrawal"
	MethodUnknown  PaymentMethod = ""
)

// MerchantInfo is the result of parsing a raw bank description.
//
// Spanish banks (Openbank in particular) do not put the merchant in
// destination_name: they leave the payee empty and encode everything in a free
// text description such as
//
//	"COMPRA EN MERCADONA, CON LA TARJETA :5412XXXXXXXX1234 FECHA 06/10/2025"
//
// Handing that string to the model verbatim wastes tokens and hides the one
// piece of signal that matters, so it is parsed into a clean merchant name.
type MerchantInfo struct {
	Merchant string        // cleaned payee name; empty when the bank gave none
	Method   PaymentMethod // payment rail inferred from the description
	Card     string        // masked card identifier (used for logging only)
}

// Key returns the canonical, case-insensitive grouping key for the merchant.
// An empty result means no merchant could be extracted.
func (m MerchantInfo) Key() string {
	return strings.ToUpper(strings.TrimSpace(m.Merchant))
}

// merchantRule is one ordered pattern applied to a raw description.
// Group 0 means "use the whole match" (e.g. bank fees, where there is no
// merchant name worth extracting).
type merchantRule struct {
	re     *regexp.Regexp
	method PaymentMethod
	group  int
}

// merchantRules are tried in order; the first match wins. Order matters: the
// more specific card-purchase form must be tested before the generic one.
var merchantRules = []merchantRule{
	// Card purchase with card suffix:
	//   "COMPRA EN AZAHAR NOCHE, SL CON LA TARJETA :5412XXXXXXXX1234"
	{regexp.MustCompile(`(?i)^COMPRA EN (.+?),?\s+CON LA TARJETA\b`), MethodCard, 1},
	// Card purchase without the card suffix: "COMPRA EN SUPERMERCADO CHINO"
	{regexp.MustCompile(`(?i)^COMPRA EN (.+)$`), MethodCard, 1},

	// Bizum used to pay a merchant: "COMPRA BIZUM VENTA BILLETES RENFE"
	{regexp.MustCompile(`(?i)^COMPRA BIZUM (.+)$`), MethodBizum, 1},
	// Bizum sent to a person: "BIZUM A FAVOR DE ALBERTO CONCEPTO: CENA"
	{regexp.MustCompile(`(?i)^BIZUM A FAVOR DE (.+)$`), MethodBizum, 1},
	{regexp.MustCompile(`(?i)^BIZUM DE (.+)$`), MethodBizum, 1},

	// Transfers: "TRANSFERENCIA INMEDIATA A FAVOR DE X CONCEPTO: Y"
	{regexp.MustCompile(`(?i)^TRANSFERENCIA(?: INMEDIATA| URGENTE)? A FAVOR DE (.+)$`), MethodTransfer, 1},
	{regexp.MustCompile(`(?i)^TRANSFERENCIA(?: INMEDIATA| URGENTE)? (?:A|HACIA) (.+)$`), MethodTransfer, 1},
	{regexp.MustCompile(`(?i)^TRANSFERENCIA(?: INMEDIATA| URGENTE)? (.+)$`), MethodTransfer, 1},

	// Bank fees carry no merchant; the whole description becomes the name so
	// every fee shares one history bucket, which is the desired behaviour.
	{regexp.MustCompile(`(?i)^COMISION\b.*$`), MethodFee, 0},
	{regexp.MustCompile(`(?i)^CUOTA\b.*$`), MethodFee, 0},
	{regexp.MustCompile(`(?i)^MANTENIMIENTO\b.*$`), MethodFee, 0},

	// Direct debits.
	{regexp.MustCompile(`(?i)^(?:RECIBO|ADEUDO|DOMICILIACION) (?:DE )?(.+)$`), MethodDirect, 1},
	{regexp.MustCompile(`(?i)^LIQUIDACION (?:DE )?(.+)$`), MethodDirect, 1},

	// Cash withdrawals.
	{regexp.MustCompile(`(?i)^(?:RETIRADA|REINTEGRO|DISPOSICION)(?: DE EFECTIVO)?(?: EN CAJERO)?\s*(.*)$`), MethodCash, 1},

	// Recurring manual payments: "PAGO DE ALQUILER", "PAGO DE CUENTAS".
	{regexp.MustCompile(`(?i)^PAGO (?:DE )?(.+)$`), MethodUnknown, 1},
}

// Trailing noise the bank appends after the merchant name. These are stripped
// from whatever the rule captured.
var (
	cardSuffixRE   = regexp.MustCompile(`(?i)\s*,?\s*CON(?: LA)? TARJETA\b.*$`)
	dateSuffixRE   = regexp.MustCompile(`(?i)\s*,?\s*FECHA:?\s*\d{1,4}[-/.]\d{1,2}[-/.]\d{1,4}.*$`)
	conceptoSuffix = regexp.MustCompile(`(?i)\s*,?\s*CONCEPTO:?.*$`)
	isoDateSuffix  = regexp.MustCompile(`(?i)\s+(?:EL\s+)?\d{4}-\d{2}-\d{2}\b.*$`)
	pedidoSuffix   = regexp.MustCompile(`(?i)\s+(?:PEDIDO|REF|AUTORIZACION)\b.*$`)
	trailingDateRE = regexp.MustCompile(`\s+\d{1,2}[/-]\d{1,2}[/-]\d{2,4}.*$`)
	trailingTimeRE = regexp.MustCompile(`\s+\d{1,2}:\d{2}(?::\d{2})?.*$`)
	cardMaskRE     = regexp.MustCompile(`(?i)\b[0-9]{0,6}[X*]{4,}[0-9]{0,6}\b`)
	legalSuffixRE  = regexp.MustCompile(`(?i)[,\s]+(?:S\.?\s?L\.?\s?U?|S\.?\s?A\.?\s?U?|S\.?\s?COOP|C\.?\s?B|SOCIEDAD LIMITADA)\.?$`)
	multiSpaceRE   = regexp.MustCompile(`\s{2,}`)
	edgeJunkRE     = regexp.MustCompile(`^[\s,;.:\-]+|[\s,;.:\-]+$`)

	// Some exports prepend the wallet that relayed the payment, e.g.
	// "Wallet openbank COMPRA EN MERCADONA, ...". It is not part of the payee.
	aggregatorPrefixRE = regexp.MustCompile(`(?i)^(?:wallet|apple pay|google pay|paypal)\s+[a-z0-9]+[\s,]+`)

	// Card aggregators append a per-transaction authorisation code:
	// "WWW.AMAZON*WB1R672N5", "AIRBNB * HMZT2MWR5C". Left in place, one merchant
	// splinters into one bucket per purchase — exactly as harmful as collapsing
	// unrelated merchants together. Codes always contain a digit, which keeps
	// real product names such as "GOOGLE*YOUTUBEPREMIUM" intact.
	authCodeRE  = regexp.MustCompile(`(?i)\s*\*\s*[A-Z0-9]*[0-9][A-Z0-9]{2,}\s*$`)
	wwwPrefixRE = regexp.MustCompile(`(?i)^www\.`)
	tldSuffixRE = regexp.MustCompile(`(?i)\.(?:com|es|net|org|app|shop|store|eu)$`)
	storeNumRE  = regexp.MustCompile(`\s+\d{3,}$`)
)

// transactionVerbs are the words a description starts with when the parser
// understands it. Used to decide whether an aggregator prefix is safe to drop.
var transactionVerbs = []string{
	"COMPRA", "BIZUM", "TRANSFERENCIA", "COMISION", "CUOTA", "MANTENIMIENTO",
	"RECIBO", "ADEUDO", "DOMICILIACION", "LIQUIDACION", "RETIRADA", "REINTEGRO",
	"DISPOSICION", "PAGO",
}

// stripAggregatorPrefix removes a leading "Wallet <provider>" marker, but only
// when what follows is still a description the parser recognises.
func stripAggregatorPrefix(s string) string {
	for i := 0; i < 2; i++ {
		stripped := aggregatorPrefixRE.ReplaceAllString(s, "")
		if stripped == s {
			return s
		}
		upper := strings.ToUpper(strings.TrimSpace(stripped))
		recognised := false
		for _, verb := range transactionVerbs {
			if strings.HasPrefix(upper, verb) {
				recognised = true
				break
			}
		}
		if !recognised {
			return s
		}
		s = stripped
	}
	return s
}

// mojibakeReplacer repairs the UTF-8-decoded-as-Latin-1 sequences that show up
// in old imported rows. Keys are unique, as strings.Replacer requires.
var mojibakeReplacer = strings.NewReplacer(
	"Ã¡", "á",
	"Ã©", "é",
	"Ã³", "ó",
	"Ãº", "ú",
	"Ã±", "ñ",
	"Ã‘", "Ñ",
	"Âº", "º",
	"Âª", "ª",
)

// ParseDescription extracts a merchant name and payment method from a raw bank
// description. It never fails: when nothing can be extracted, Merchant is
// empty and Method is MethodUnknown.
func ParseDescription(description string) MerchantInfo {
	raw := stripAggregatorPrefix(strings.TrimSpace(description))
	if raw == "" {
		return MerchantInfo{}
	}

	info := MerchantInfo{}
	rest := ""

	for _, rule := range merchantRules {
		m := rule.re.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		info.Method = rule.method
		if rule.group == 0 || rule.group >= len(m) {
			rest = m[0]
		} else {
			rest = m[rule.group]
		}
		break
	}

	if rest == "" {
		// No rule matched: the description is probably already clean
		// ("LA CUEVA DEL BARBERO", "TAXI RIDE"). Keep it as the merchant.
		rest = raw
	}

	info.Card = cardMaskRE.FindString(raw)
	info.Merchant = cleanMerchant(rest)
	return info
}

// Redact returns the raw description with the masked card number and the
// boilerplate tail removed. The model never needs the card mask or the
// duplicated date, and sending them only burns tokens.
func Redact(description string) string {
	s := stripAggregatorPrefix(strings.TrimSpace(description))
	if s == "" {
		return ""
	}
	s = cardSuffixRE.ReplaceAllString(s, "")
	s = dateSuffixRE.ReplaceAllString(s, "")
	s = conceptoSuffix.ReplaceAllString(s, "")
	s = isoDateSuffix.ReplaceAllString(s, "")
	s = pedidoSuffix.ReplaceAllString(s, "")
	s = cardMaskRE.ReplaceAllString(s, "")
	s = trailingDateRE.ReplaceAllString(s, "")
	s = trailingTimeRE.ReplaceAllString(s, "")
	s = multiSpaceRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// cleanMerchant strips the trailing reference noise, legal-form suffixes and
// mojibake from a candidate merchant name.
func cleanMerchant(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	s = cardSuffixRE.ReplaceAllString(s, "")
	s = dateSuffixRE.ReplaceAllString(s, "")
	s = conceptoSuffix.ReplaceAllString(s, "")
	s = isoDateSuffix.ReplaceAllString(s, "")
	s = pedidoSuffix.ReplaceAllString(s, "")
	s = trailingDateRE.ReplaceAllString(s, "")
	s = trailingTimeRE.ReplaceAllString(s, "")
	s = cardMaskRE.ReplaceAllString(s, "")
	s = authCodeRE.ReplaceAllString(s, "")
	s = wwwPrefixRE.ReplaceAllString(s, "")
	s = tldSuffixRE.ReplaceAllString(s, "")
	s = storeNumRE.ReplaceAllString(s, "")
	s = multiSpaceRE.ReplaceAllString(s, " ")

	// Legal forms are stripped repeatedly so "X, S.L.U." and "X SL" converge.
	for i := 0; i < 3; i++ {
		next := legalSuffixRE.ReplaceAllString(s, "")
		if next == s {
			break
		}
		s = next
	}

	s = edgeJunkRE.ReplaceAllString(s, "")
	s = repairMojibake(s)
	s = multiSpaceRE.ReplaceAllString(s, " ")

	return strings.TrimSpace(s)
}

// repairMojibake fixes the character corruption found in historical rows.
func repairMojibake(s string) string {
	s = mojibakeReplacer.Replace(s)
	if strings.ContainsRune(s, '¥') {
		s = repairBrokenEnye(s)
	}
	return s
}

// repairBrokenEnye rewrites a '¥' that sits *inside* a word back to 'ñ' (or 'Ñ'
// when the surrounding letters are upper-case). Some Openbank rows encode "ñ" as
// 0xA5, which decodes as ¥; a genuine yen amount ("¥1500") is never surrounded
// by letters, so it is left untouched.
func repairBrokenEnye(s string) string {
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range rs {
		if r == '¥' && i > 0 && i < len(rs)-1 && unicode.IsLetter(rs[i-1]) && unicode.IsLetter(rs[i+1]) {
			if unicode.IsUpper(rs[i-1]) {
				b.WriteRune('Ñ')
			} else {
				b.WriteRune('ñ')
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsUsableAccountName reports whether an expense-account name is a real payee.
// Placeholder accounts (e.g. "(unknown destination)", "(no name)") must never
// reach the model, and must never be re-used as a destination, because doing
// so silently collapses unrelated merchants into a single "vendor".
func IsUsableAccountName(name string) bool {
	return !isBlankName(name)
}

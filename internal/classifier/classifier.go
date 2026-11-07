package classifier

import (
	"context"
	"strings"
)

type Outcome string

const (
	Classified  Outcome = "CLASSIFIED"
	Assumed     Outcome = "ASSUMED"
	NeedsReview Outcome = "NEEDS_REVIEW"
)

type HistoricalEntry struct {
	TransactionID        string
	DestinationName      string
	Description          string
	CategoryName         string
	GroupKey             string  // effective lookup key — destination name, or description when destination is blank
	Amount               float64 // 0 means unknown; used for history-match confidence check
	DestinationAccountID string  // expense account ID from a previous classification (empty if unknown)
}

// GroupKey returns the field to use when grouping and looking up transaction
// history. A real destination name wins, because that is the payee the user or
// Firefly III already agreed on. When destination_name is absent or a generic
// placeholder, the merchant is parsed out of the bank description instead —
// Spanish banks such as Openbank never populate destination_name and put the
// whole transaction into the description. Only when neither yields anything is
// the raw description used, so past context is still surfaced.
func GroupKey(destinationName, description string) string {
	if !isBlankName(destinationName) {
		return strings.TrimSpace(destinationName)
	}
	if key := ParseDescription(description).Key(); key != "" {
		return key
	}
	return strings.TrimSpace(description)
}

// blankNames are the placeholder payee names Firefly III and the various bank
// importers use when no real merchant is known. Grouping on any of these
// collapses unrelated payees into a single bucket of poisoned "history", so
// they must never be treated as a merchant.
var blankNames = map[string]bool{
	"":                      true,
	"-":                     true,
	"--":                    true,
	"no name":               true,
	"(no name)":             true,
	"unknown":               true,
	"n/a":                   true,
	"n.a.":                  true,
	"unknown payee":         true,
	"(unknown payee)":       true,
	"(no payee)":            true,
	"(unknown destination)": true,
	"unknown destination":   true,
	"pos purchase":          true,
	"pos purchase (card)":   true,
	"card purchase":         true,
	"purchase":              true,
	"transfer":              true,
	"(transfer)":            true,
	"no registrado":         true,
	"(no registrado)":       true,
}

func isBlankName(s string) bool {
	norm := strings.ToLower(strings.TrimSpace(s))
	if blankNames[norm] {
		return true
	}
	// Catch decorated variants such as "(unknown destination) #12".
	return strings.Contains(norm, "unknown destination")
}

// Category is a classifiable Firefly III category with optional notes that
// provide the LLM with extra context about what belongs in each category.
type Category struct {
	Name  string
	Notes string
}

// AccountCandidate is an expense account that the LLM can match or decide to create.
type AccountCandidate struct {
	ID   string
	Name string
}

// DestinationResult captures the LLM's decision about the destination account.
type DestinationResult struct {
	Name       string // desired account name
	Action     string // "MATCH" or "CREATE"
	Confidence string // "CLASSIFIED" or "ASSUMED"
}

type Request struct {
	Categories      []Category
	DestinationName string
	Description     string
	Amount          *float64
	History         []HistoricalEntry

	// Merchant and Method optionally carry an already-parsed merchant name and
	// payment method. When empty, buildUserPrompt derives them from
	// Description via ParseDescription, so callers that only have the raw
	// description (e.g. the webhook path) need not do the work themselves.
	Merchant string
	Method   string

	// ExpenseAccounts is set when destination-account matching is enabled.
	ExpenseAccounts []AccountCandidate

	// DestinationMatching signals to the classifier that the system prompt
	// and response parsing should include destination-account logic.
	DestinationMatching bool

	// CategoryOnly signals that only category classification is needed
	// (destination info is stripped from the prompt). When true, the classifier
	// won't bother computing a destination even if expense accounts are provided.
	CategoryOnly bool

	// SystemPromptOverride replaces the built-in system prompt when non-empty.
	// Used for special-purpose LLM calls like transfer destination suggestion.
	SystemPromptOverride string
}

type Result struct {
	Outcome     Outcome
	Category    string // empty if NeedsReview
	Reason      string
	Assumption  string // non-empty only when Assumed
	RawPrompt   string
	RawResponse string

	// Destination is set when destination-account matching was enabled
	// and the LLM returned a valid destination suggestion.
	Destination *DestinationResult
}

type Classifier interface {
	Classify(ctx context.Context, req Request) (Result, error)
}

package prorata

// RuleID identifies the billing rule that produced an invoice line.
// An invoice line that cannot explain itself is a bug by definition, so the
// engine rejects lines with an empty RuleID or Description.
type RuleID string

// Line is a single charge (positive) or credit (negative) on an invoice.
type Line struct {
	RuleID      RuleID `json:"rule"`
	Description string `json:"description"`
	Amount      Money  `json:"amount_cents"`
}

// Invoice is the result of a billing computation: an explained list of
// charges and credits and their total.
type Invoice struct {
	Currency string `json:"currency"`
	Lines    []Line `json:"lines,omitempty"`
	Total    Money  `json:"total_cents"`
}

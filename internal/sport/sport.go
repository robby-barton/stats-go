// Package sport defines the canonical sport identifiers used for persistence
// ("ncaaf"/"ncaam"/"ncaaw"), shared by the database models, ranking, updater,
// and CLI wiring. ESPN-specific slugs and URLs stay inside the espn package.
package sport

import "fmt"

// Sport is the persistence identifier for a college sport.
type Sport string

const (
	Football Sport = "ncaaf"

	// Men's and women's college basketball share one ranking/updater behavior
	// on dedicated ESPN paths; only the persistence key differs.
	Basketball       Sport = "ncaam"
	WomensBasketball Sport = "ncaaw"
)

// IsBasketball reports whether s is a men's or women's basketball sport.
// Both basketball sports share the single-division (D-I only) ranking and
// season paths; only the ESPN endpoints and persistence key differ.
func (s Sport) IsBasketball() bool {
	return s == Basketball || s == WomensBasketball
}

// Validate reports whether s is a known sport. Callers should validate at
// construction time so no downstream code has to handle unknown values.
func (s Sport) Validate() error {
	switch s {
	case Football, Basketball, WomensBasketball:
		return nil
	default:
		return fmt.Errorf("unknown sport %q (want %q, %q, or %q)",
			string(s), Football, Basketball, WomensBasketball)
	}
}

// Parse converts a raw string into a Sport, rejecting unknown values.
func Parse(s string) (Sport, error) {
	parsed := Sport(s)
	if err := parsed.Validate(); err != nil {
		return "", err
	}
	return parsed, nil
}

// Package sport defines the canonical sport identifiers used for persistence
// ("ncaaf"/"ncaam"), shared by the database models, ranking, updater, and CLI
// wiring. ESPN-specific slugs and URLs stay inside the espn package.
package sport

import "fmt"

// Sport is the persistence identifier for a college sport.
type Sport string

const (
	Football   Sport = "ncaaf"
	Basketball Sport = "ncaam"
)

// Validate reports whether s is a known sport. Callers should validate at
// construction time so no downstream code has to handle unknown values.
func (s Sport) Validate() error {
	switch s {
	case Football, Basketball:
		return nil
	default:
		return fmt.Errorf("unknown sport %q (want %q or %q)", string(s), Football, Basketball)
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

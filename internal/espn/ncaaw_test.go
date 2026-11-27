package espn

import (
	"testing"

	"github.com/robby-barton/stats-go/internal/sport"
)

// TestNewClientForSport_WomensBasketball verifies the women's basketball
// client wiring: the dedicated ESPN path, the shared basketball client
// behavior, the D-I group 50, and the "ncaaw" persistence key.
func TestNewClientForSport_WomensBasketball(t *testing.T) {
	client := NewClientForSport(CollegeWomensBasketball)

	bc, ok := client.(*BasketballClient)
	if !ok {
		t.Fatalf("NewClientForSport(CollegeWomensBasketball) = %T, want *BasketballClient", client)
	}
	if bc.Sport != CollegeWomensBasketball {
		t.Errorf("Sport = %q, want %q", bc.Sport, CollegeWomensBasketball)
	}
	if got := bc.SportInfo().DBSport(); got != sport.WomensBasketball {
		t.Errorf("DBSport = %q, want %q", got, sport.WomensBasketball)
	}

	// The women's client points at the dedicated ESPN URL segment while
	// sharing the men's endpoint shapes and D1 group 50.
	urls := SportURLs(CollegeWomensBasketball)
	for name, url := range map[string]string{
		"GameStats":   urls.GameStats,
		"TeamInfo":    urls.TeamInfo,
		"Scoreboard":  urls.Scoreboard,
		"Conferences": urls.Conferences,
	} {
		if url == "" {
			t.Errorf("%s URL is empty", name)
			continue
		}
		if !containsAll(url, "site.api.espn.com", "basketball/womens-college-basketball") {
			t.Errorf("%s URL %q must carry the women's path", name, url)
		}
	}

	// Both basketball sports resolve to D1 group 50 on their own paths.
	for _, sp := range []Sport{CollegeBasketball, CollegeWomensBasketball} {
		groups := sp.Groups()
		if len(groups) != 1 || groups[0] != D1Basketball {
			t.Errorf("%q Groups = %v, want [50]", sp, groups)
		}
		if sp.IsBasketball() == false || sp.HasDivisionSplit() {
			t.Errorf("%q: IsBasketball false or HasDivisionSplit true", sp)
		}
	}

	// The men's URLs must not have been repointed by the women's mapping.
	mens := SportURLs(CollegeBasketball)
	if !containsAll(mens.Scoreboard, "mens-college-basketball") {
		t.Errorf("men's scoreboard URL %q lost its men's path", mens.Scoreboard)
	}
	if containsAll(mens.Scoreboard, "womens-college-basketball") {
		t.Errorf("men's scoreboard URL %q must not use the women's path", mens.Scoreboard)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

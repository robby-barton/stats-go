package fbsprior

import (
	"errors"
	"fmt"
	"time"
)

// The fixed preseason anchor policy, deliberately distinct from the former
// AP-date anchor: the anchor is always midnight Eastern on August 10 of the
// season year, and the day-56 calendar expiry is derived from that fixed
// anchor — never from the actual capture date. Capture may happen on any
// August 10-16 Eastern date (the bounded retry window); the anchor, and
// therefore the expiry, are the same for every capture date of a season.
const (
	// AnchorMonth and AnchorDay fix the preseason anchor in Eastern time.
	AnchorMonth = time.August
	AnchorDay   = 10
	// MaxFreezeDay is the last Eastern calendar day of the bounded freeze
	// window: attempts may capture on August 10 through MaxFreezeDay.
	MaxFreezeDay = 16
)

// AnchorInstantFor returns midnight Eastern on August 10 of the season year,
// normalized to UTC.
func AnchorInstantFor(season int64) (time.Time, error) {
	if season <= 0 {
		return time.Time{}, fmt.Errorf("fbsprior: season %d must be positive", season)
	}
	return easternMidnight(fmt.Sprintf("%04d-%02d-%02d", season, int(AnchorMonth), AnchorDay))
}

// CalendarExpiryForSeason returns the fixed full-bypass instant for a season:
// midnight Eastern on August 10 of the season year plus CalendarDays Eastern
// calendar days. Callers use it to skip stored-row reads once the blend can
// never apply again.
func CalendarExpiryForSeason(season int64) (time.Time, error) {
	anchor, err := AnchorInstantFor(season)
	if err != nil {
		return time.Time{}, err
	}
	return anchor.AddDate(0, 0, CalendarDays), nil
}

// SeasonWindow resolves the freeze policy for an observed instant: the
// Eastern calendar year as the target season, the observed Eastern capture
// date, and whether that date lies within the bounded August 10-16 Eastern
// window. The zone lookup never fails for well-formed tz data; if it does,
// the window is reported closed so no attempt runs outside Eastern time.
func SeasonWindow(now time.Time) (int64, string, bool) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return 0, "", false
	}
	et := now.In(loc)
	season := int64(et.Year())
	freezeDate := et.Format(time.DateOnly)
	inWindow := et.Month() == AnchorMonth &&
		et.Day() >= AnchorDay && et.Day() <= MaxFreezeDay
	return season, freezeDate, inWindow
}

// ValidateSeasonWindow re-derives the anchor for a season and reports
// whether the given observed capture date is a valid August 10-16 Eastern
// date of that season. It is the freeze-time recheck: loading and strength
// computation may take time, so the window is verified again immediately
// before persisting.
func ValidateSeasonWindow(season int64, freezeDate string) error {
	instant, err := easternMidnight(freezeDate)
	if err != nil {
		return fmt.Errorf("fbsprior: freeze date %q: %w", freezeDate, err)
	}
	if instant.Month() != AnchorMonth ||
		instant.Day() < AnchorDay || instant.Day() > MaxFreezeDay ||
		int64(instant.Year()) != season {
		return errors.New("fbsprior: freeze window closed at capture (must be August 10-16 Eastern of the season year)")
	}
	return nil
}

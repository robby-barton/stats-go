package updater

import (
	"context"
	"fmt"
	"maps"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/team"
)

func (u *Updater) insertSeasonToDB(seasons []database.TeamSeason) error {
	return u.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.OnConflict{
				UpdateAll: true, // upsert
			}).
			CreateInBatches(seasons, 1000).Error; err != nil {
			return err
		}

		return nil
	})
}

func (u *Updater) seasonsExist(year int64) (bool, error) {
	var count int64
	err := u.db.Model(database.TeamSeason{}).Where("sport = ? and year = ?", u.sportDB(), year).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateTeamSeasons updates team season records for the current ESPN season.
func (u *Updater) UpdateTeamSeasons(ctx context.Context, force bool) (int, error) {
	currentSeason, err := u.espn.DefaultSeason(ctx)
	if err != nil {
		return 0, err
	}
	return u.updateTeamSeasonsForYear(ctx, currentSeason, force)
}

// UpdateTeamSeasonsForYear updates team season records for a specific year.
// Use force=true to overwrite existing records.
func (u *Updater) UpdateTeamSeasonsForYear(ctx context.Context, year int64, force bool) (int, error) {
	return u.updateTeamSeasonsForYear(ctx, year, force)
}

func (u *Updater) updateTeamSeasonsForYear(ctx context.Context, year int64, force bool) (int, error) {
	if !force {
		exists, err := u.seasonsExist(year)
		if err != nil {
			return 0, fmt.Errorf("checking existing team seasons for %d: %w", year, err)
		}
		if exists {
			u.logger.Info("Not updating")
			return 0, nil
		}
	}

	sport := u.sportDB()

	teamConfs, err := u.espn.TeamConferencesByYear(ctx, year)
	if err != nil {
		return 0, err
	}

	var teamSeasons []database.TeamSeason

	confResult, err := u.espn.ConferenceMap(ctx)
	if err != nil {
		return 0, err
	}

	if u.espn.SportInfo() == espn.CollegeBasketball {
		// Basketball: all D1 teams are top-division (FBS=1). Conference names
		// come from the conference API but there's no FBS/FCS split.
		d1Confs := confResult.Conferences[espn.D1Basketball]

		for team, conf := range teamConfs {
			confName, ok := d1Confs[conf]
			if !ok {
				continue // skip non-D1 teams (e.g. D2/D3/NAIA opponents)
			}
			teamSeasons = append(teamSeasons, database.TeamSeason{
				TeamID: team,
				Conf:   confName,
				Year:   year,
				Sport:  sport,
				FBS:    1, // all D1 basketball teams treated as top-division
			})
		}
	} else {
		fbs := confResult.Conferences[espn.FBS]
		fbsfcs := maps.Clone(fbs)
		maps.Copy(fbsfcs, confResult.Conferences[espn.FCS])

		for team, conf := range teamConfs {
			confName, ok := fbsfcs[conf]
			if !ok {
				continue
			}
			var isFBS int64
			if _, ok := fbs[conf]; ok {
				isFBS = 1
			}
			teamSeasons = append(teamSeasons, database.TeamSeason{
				TeamID: team,
				Conf:   confName,
				Year:   year,
				Sport:  sport,
				FBS:    isFBS,
			})
		}
	}

	if err := u.insertSeasonToDB(teamSeasons); err != nil {
		return 0, err
	}

	// Season-discovered teams may be absent from team_names: ESPN's bulk
	// /teams endpoint omits some teams (e.g. recent D-I transition schools
	// like Southern Indiana, Queens, Lindenwood). Backfill identity rows
	// from the site.api per-team endpoint so rankings render names/logos.
	if created, err := u.backfillMissingTeamNames(ctx, year); err != nil {
		u.logger.Errorf("team name backfill for %d: %v", year, err)
	} else if created > 0 {
		u.logger.Infof("backfilled %d missing team_names rows", created)
	}

	return len(teamSeasons), nil
}

// backfillMissingTeamNames creates team_names rows for teams that have a
// team_seasons row for the given year but no team_names entry. Identity
// data comes from the site.api per-team endpoint; one failing fetch is
// logged and skipped rather than aborting the season update.
func (u *Updater) backfillMissingTeamNames(ctx context.Context, year int64) (int, error) {
	var ids []int64
	err := u.db.Raw(`select distinct ts.team_id
		from team_seasons ts
		where ts.sport = ? and ts.year = ?
		  and not exists (select 1 from team_names n
		                  where n.team_id = ts.team_id and n.sport = ts.sport)`,
		u.sportDB(), year).Scan(&ids).Error
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	created := 0
	for _, id := range ids {
		detail, err := u.espn.GetTeamByID(ctx, id)
		if err != nil {
			u.logger.Errorf("team name backfill: fetching team %d: %v", id, err)
			continue
		}
		rows := apiToDB([]team.ParsedTeamInfo{team.ParsedFromESPN(detail.Team)})
		for i := range rows {
			rows[i].Sport = u.sportDB()
		}
		if err := u.insertTeamsToDB(rows); err != nil {
			u.logger.Errorf("team name backfill: inserting team %d: %v", id, err)
			continue
		}
		created++
	}
	return created, nil
}

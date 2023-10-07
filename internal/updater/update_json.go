package updater

import (
	"context"
	"fmt"
	"strconv"

	"github.com/robby-barton/stats-go/internal/database"
)

const (
	fbs = "fbs"
	fcs = "fcs"
)

func (u *Updater) UpdateAvailRanksJSON() error {
	years := []struct {
		Year       int64 `json:"-"`
		Weeks      int64 `json:"weeks"`
		Postseason int64 `json:"postseason"`
	}{}
	if err := u.DB.Model(&database.TeamWeekResult{}).
		Select("year, max(case when postseason = 0 then week else 0 end) as weeks, max(postseason) as postseason").
		Group("year").Order("year").Scan(&years).Error; err != nil {
		return err
	}

	availRanks := map[int64]interface{}{}
	for _, year := range years {
		availRanks[year.Year] = year
	}

	return u.Writer.WriteData(context.Background(), "availRanks.json", availRanks)
}

type teamJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (u *Updater) UpdateTeamJSON() error {
	teams := []teamJSON{}
	if err := u.DB.Model(&database.TeamName{}).
		Select("team_id as id, name").
		Scan(&teams).Error; err != nil {
		return err
	}

	teamMap := map[int64]teamJSON{}
	for _, team := range teams {
		teamMap[team.ID] = team
	}

	return u.Writer.WriteData(context.Background(), "teams.json", teamMap)
}

type rankingsJSON struct {
	Division   string                    `json:"division"`
	Year       int64                     `json:"year"`
	Week       int64                     `json:"week"`
	Postseason bool                      `json:"postseason"`
	Results    []database.TeamWeekResult `json:"results"`
}

func (u *Updater) UpdateRankJSON(week *rankingsJSON) error {
	weekName := "final"
	if !week.Postseason {
		weekName = strconv.FormatInt(week.Week, 10)
	}
	fileName := fmt.Sprintf("ranking/%d/%s/%s.json", week.Year, week.Division, weekName)
	return u.Writer.WriteData(context.Background(), fileName, week.Results)
}

func (u *Updater) UpdateTeamRankJSON(team int64) error {
	teamRankings := []database.TeamWeekResult{}
	if err := u.DB.Where(
		"team_id = ?", team,
	).Order("year desc, postseason desc, week desc").Find(&teamRankings).Error; err != nil {
		return err
	}

	fileName := fmt.Sprintf("team/%d.json", team)
	return u.Writer.WriteData(context.Background(), fileName, teamRankings)
}

func (u *Updater) UpdateAllJSON() error {
	if err := u.UpdateAvailRanksJSON(); err != nil {
		return err
	}

	if err := u.UpdateTeamJSON(); err != nil {
		return err
	}

	teams := []int64{}
	if err := u.DB.Model(&database.TeamWeekResult{}).
		Distinct("team_id").Pluck("team_id", &teams).Error; err != nil {
		return err
	}
	for _, team := range teams {
		if err := u.UpdateTeamRankJSON(team); err != nil {
			return err
		}
	}

	yearInfo, err := u.getYearInfo()
	if err != nil {
		return err
	}

	for _, division := range []string{fbs, fcs} {
		for _, year := range yearInfo {
			for week := int64(1); week <= year.Weeks; week++ {
				weekRankings := []database.TeamWeekResult{}
				if err := u.DB.Where(
					"year = ? and fbs = ? and week = ? and postseason = ?",
					year.Year, division == fbs, week, 0,
				).
					Order("final_rank").
					Find(&weekRankings).Error; err != nil {
					return err
				}
				err = u.UpdateRankJSON(&rankingsJSON{
					Division:   division,
					Year:       year.Year,
					Week:       week,
					Postseason: false,
					Results:    weekRankings,
				})
				if err != nil {
					return err
				}
			}

			if year.Postseason > 0 {
				weekRankings := []database.TeamWeekResult{}
				if err := u.DB.Where(
					"year = ? and fbs = ? and postseason = ?",
					year.Year, division == fbs, 1,
				).
					Order("final_rank").
					Find(&weekRankings).Error; err != nil {
					return err
				}

				err := u.UpdateRankJSON(&rankingsJSON{
					Division:   division,
					Year:       year.Year,
					Week:       1,
					Postseason: true,
					Results:    weekRankings,
				})
				if err != nil {
					return err
				}
			} else {
				weekRankings := []database.TeamWeekResult{}
				if err := u.DB.Where(
					"year = ? and fbs = ? and week = ? and postseason = ?",
					year.Year, division == fbs, year.Weeks+1, 0,
				).
					Order("final_rank").
					Find(&weekRankings).Error; err != nil {
					return err
				}

				err := u.UpdateRankJSON(&rankingsJSON{
					Division:   division,
					Year:       year.Year,
					Week:       year.Weeks + 1,
					Postseason: false,
					Results:    weekRankings,
				})
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

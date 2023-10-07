package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/robby-barton/stats-go/internal/database"
)

const (
	fbs = "fbs"
	fcs = "fcs"
)

type Writer interface {
	WriteData(ctx context.Context, fileName string, data []byte) error
}

type RankingsJSON struct {
	Division   string                    `json:"division"`
	Year       int64                     `json:"year"`
	Week       int64                     `json:"week"`
	Postseason bool                      `json:"postseason"`
	Results    []database.TeamWeekResult `json:"results"`
}

type DefaultWriter struct{}

func (*DefaultWriter) WriteData(_ context.Context, fileName string, data []byte) error {
	err := os.MkdirAll(filepath.Dir(fileName), 0775)
	if err != nil {
		return err
	}

	return os.WriteFile(fileName, data, 0664) // #nosec G306
}

func (u *Updater) UpdateTeamJSON() error {
	var teams []database.TeamName
	err := u.DB.Find(&teams).Error
	if err != nil {
		return err
	}

	teamMap := map[int64]database.TeamName{}
	for _, team := range teams {
		teamMap[team.TeamID] = team
	}

	data, err := json.MarshalIndent(teamMap, "", "    ")
	if err != nil {
		return err
	}
	err = u.Writer.WriteData(context.Background(), "teams.json", data)
	if err != nil {
		return err
	}

	return nil
}

func (u *Updater) UpdateRankJSON(week *RankingsJSON) error {
	data, err := json.MarshalIndent(week.Results, "", "    ")
	if err != nil {
		return err
	}

	weekName := "final"
	if !week.Postseason {
		weekName = strconv.FormatInt(week.Week, 10)
	}
	fileName := fmt.Sprintf("ranking/%d/%s/%s.json", week.Year, week.Division, weekName)
	return u.Writer.WriteData(context.Background(), fileName, data)
}

func (u *Updater) UpdateAllJSON() error {
	err := u.UpdateTeamJSON()
	if err != nil {
		return err
	}

	yearInfo, err := u.getYearInfo()
	if err != nil {
		return err
	}

	var rankings []database.TeamWeekResult
	err = u.DB.Find(&rankings).Error
	if err != nil {
		return err
	}

	for _, division := range []string{fbs, fcs} {
		for _, year := range yearInfo {
			for week := int64(1); week <= year.Weeks; week++ {
				var weekRankings []database.TeamWeekResult
				if err := u.DB.Where(
					"year = ? and fbs = ? and week = ? and postseason = ?",
					year.Year, division == fbs, week, 0,
				).
					Order("final_rank").
					Find(&weekRankings).Error; err != nil {
					return err
				}
				err = u.UpdateRankJSON(&RankingsJSON{
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
				var weekRankings []database.TeamWeekResult
				if err := u.DB.Where(
					"year = ? and fbs = ? and postseason = ?",
					year.Year, division == fbs, 1,
				).
					Order("final_rank").
					Find(&weekRankings).Error; err != nil {
					return err
				}

				err := u.UpdateRankJSON(&RankingsJSON{
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
				var weekRankings []database.TeamWeekResult
				if err := u.DB.Where(
					"year = ? and fbs = ? and week = ? and postseason = ?",
					year.Year, division == fbs, year.Weeks+1, 0,
				).
					Order("final_rank").
					Find(&weekRankings).Error; err != nil {
					return err
				}

				err := u.UpdateRankJSON(&RankingsJSON{
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

package updater

import (
	"context"
	"fmt"
	"slices"
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
	ID       int64  `json:"team_id"`
	Name     string `json:"name"`
	Logo     string `json:"logo"`
	LogoDark string `json:"logo_dark"`
}

func (u *Updater) getTeamInfo() (map[int64]teamJSON, error) {
	teams := []teamJSON{}
	if err := u.DB.Model(&database.TeamName{}).
		Select("team_id as id, name, logo, logo_dark").
		Scan(&teams).Error; err != nil {
		return nil, err
	}

	teamMap := map[int64]teamJSON{}
	for _, team := range teams {
		teamMap[team.ID] = team
	}

	return teamMap, nil
}

func (u *Updater) UpdateTeamsJSON(teamMap map[int64]teamJSON) error {
	teamList := []teamJSON{}
	if teamMap != nil {
		for _, team := range teamMap {
			teamList = append(teamList, team)
		}
	} else {
		if err := u.DB.Model(&database.TeamName{}).
			Select("team_id as id, name, logo, logo_dark").
			Scan(&teamList).Error; err != nil {
			return err
		}
	}

	return u.Writer.WriteData(context.Background(), "teams.json", teamList)
}

type rankingsJSON struct {
	Division   string        `json:"division"`
	Year       int64         `json:"year"`
	Week       int64         `json:"week"`
	Postseason bool          `json:"postseason"`
	Results    []*resultJSON `json:"results"`
}

type resultJSON struct {
	Team      teamJSON `json:"team"`
	FinalRank int64    `json:"final_rank"`
	Conf      string   `json:"conf"`
	Record    string   `json:"record"`
	SRSRank   int64    `json:"srs_rank"`
	SOSRank   int64    `json:"sos_rank"`
	FinalRaw  float64  `json:"final_raw"`
}

func toJSON(rank *database.TeamWeekResult, teamMap map[int64]teamJSON) *resultJSON {
	record := fmt.Sprintf("%d-%d", rank.Wins, rank.Losses)
	if rank.Ties > 0 {
		record = fmt.Sprintf("%d-%d-%d", rank.Week, rank.Losses, rank.Ties)
	}
	return &resultJSON{
		Team:      teamMap[rank.TeamID],
		FinalRank: rank.FinalRank,
		Conf:      rank.Conf,
		Record:    record,
		SRSRank:   rank.SRSRank,
		SOSRank:   rank.SOSRank,
		FinalRaw:  rank.FinalRaw,
	}
}

func (u *Updater) UpdateRankJSON(week *rankingsJSON) error {
	weekName := "final"
	if !week.Postseason {
		weekName = strconv.FormatInt(week.Week, 10)
	}
	fileName := fmt.Sprintf("ranking/%d/%s/%s.json", week.Year, week.Division, weekName)
	return u.Writer.WriteData(context.Background(), fileName, week.Results)
}

func (u *Updater) UpdateIndexJSON(week *rankingsJSON) error {
	return u.Writer.WriteData(context.Background(), "latest.json", week.Results)
}

type teamRankJSON struct {
	Team     teamJSON       `json:"team"`
	RankList []teamRankList `json:"rank_list"`
	Years    []int64        `json:"years"`
}

type teamRankList struct {
	Week      string `json:"week"`
	Rank      int64  `json:"rank"`
	FillLevel int64  `json:"fill_level"`
}

func (u *Updater) UpdateTeamRankJSON(team teamJSON) error {
	teamRankings := []database.TeamWeekResult{}
	if err := u.DB.Model(&database.TeamWeekResult{}).Where(
		"team_id = ?", team.ID,
	).Order("year, postseason, week").Find(&teamRankings).Error; err != nil {
		return err
	}

	years := []int64{}
	teamRanks := []teamRankList{}
	for _, rank := range teamRankings {
		if !slices.Contains(years, rank.Year) {
			years = append(years, rank.Year)
		}
		week := fmt.Sprintf("%d Week %d", rank.Year, rank.Week)
		if rank.Postseason > 0 {
			week = fmt.Sprintf("%d Final", rank.Year)
		}
		teamRanks = append(teamRanks, teamRankList{
			Week:      week,
			Rank:      rank.FinalRank,
			FillLevel: 150,
		})
	}

	results := &teamRankJSON{
		Team:     team,
		RankList: teamRanks,
		Years:    years,
	}

	fileName := fmt.Sprintf("team/%d.json", team.ID)
	return u.Writer.WriteData(context.Background(), fileName, results)
}

type gameCountJSON struct {
	Team  teamJSON `json:"team"`
	Sun   int64    `json:"sun"`
	Mon   int64    `json:"mon"`
	Tue   int64    `json:"tue"`
	Wed   int64    `json:"wed"`
	Thu   int64    `json:"thu"`
	Fri   int64    `json:"fri"`
	Sat   int64    `json:"sat"`
	Total int64    `json:"total"`
}

func (u *Updater) UpdateGameCountJSON(teamMap map[int64]teamJSON) error {
	sql := `
	with gamesList as (
		(
			select
				home_id as team_id,
				extract(dow from start_time) as dow,
				game_id
			from games
		) union all (
			select
				away_id as team_id,
				extract(dow from start_time) as dow,
				game_id
			from games
		)
	)
	select
		team_id,
		sum(case when dow = 0 then 1 else 0 end) as sun,
		sum(case when dow = 1 then 1 else 0 end) as mon,
		sum(case when dow = 2 then 1 else 0 end) as tue,
		sum(case when dow = 3 then 1 else 0 end) as wed,
		sum(case when dow = 4 then 1 else 0 end) as thu,
		sum(case when dow = 5 then 1 else 0 end) as fri,
		sum(case when dow = 6 then 1 else 0 end) as sat,
		count(1) as total
	from gamesList
	group by
		team_id
	order by
		total desc
	`

	results := []struct {
		TeamID int64
		Sun    int64
		Mon    int64
		Tue    int64
		Wed    int64
		Thu    int64
		Fri    int64
		Sat    int64
		Total  int64
	}{}

	if err := u.DB.Raw(sql).Scan(&results).Error; err != nil {
		return err
	}

	resultsJSON := []*gameCountJSON{}
	for _, result := range results {
		team, ok := teamMap[result.TeamID]
		if !ok {
			continue
		}
		resultsJSON = append(resultsJSON, &gameCountJSON{
			Team:  team,
			Sun:   result.Sun,
			Mon:   result.Mon,
			Tue:   result.Tue,
			Wed:   result.Wed,
			Thu:   result.Thu,
			Fri:   result.Fri,
			Sat:   result.Sat,
			Total: result.Total,
		})
	}

	return u.Writer.WriteData(context.Background(), "gameCount.json", resultsJSON)
}

func (u *Updater) UpdateRecentJSON() error {
	teamMap, err := u.getTeamInfo()
	if err != nil {
		return err
	}

	if err := u.UpdateAvailRanksJSON(); err != nil {
		return err
	}

	if err := u.UpdateGameCountJSON(teamMap); err != nil {
		return err
	}

	sql := `
	select 
		max(year) as year,
		max(week) as week,
		max(postseason) as postseason 
	from team_week_results 
	where 
		year = (
			select 
				max(year) 
			from team_week_results
		)
	`
	yearInfo := &struct {
		Year       int64
		Week       int64
		Postseason int64
	}{}
	if err := u.DB.Raw(sql).Scan(yearInfo).Error; err != nil {
		return err
	}

	teams := []int64{}
	if err := u.DB.Model(&database.TeamWeekResult{}).
		Distinct("team_id").Where("year = ?", yearInfo.Year).Pluck("team_id", &teams).Error; err != nil {
		return err
	}
	for _, team := range teams {
		if err := u.UpdateTeamRankJSON(teamMap[team]); err != nil {
			return err
		}
	}

	for _, division := range []string{fbs, fcs} {
		weekRankings := []database.TeamWeekResult{}
		if yearInfo.Postseason > 0 {
			if err := u.DB.Where(
				"year = ? and fbs = ? and postseason = 1",
				yearInfo.Year, division == fbs,
			).
				Order("final_rank").
				Find(&weekRankings).Error; err != nil {
				return err
			}
		} else {
			if err := u.DB.Where(
				"year = ? and fbs = ? and week = ? and postseason = 0",
				yearInfo.Year, division == fbs, yearInfo.Week,
			).
				Order("final_rank").
				Find(&weekRankings).Error; err != nil {
				return err
			}
		}

		weekJSON := []*resultJSON{}
		for _, week := range weekRankings {
			temp := week
			weekJSON = append(weekJSON, toJSON(&temp, teamMap))
		}
		json := &rankingsJSON{
			Division:   division,
			Year:       yearInfo.Year,
			Week:       yearInfo.Week,
			Postseason: yearInfo.Postseason > 0,
			Results:    weekJSON,
		}
		if err := u.UpdateRankJSON(json); err != nil {
			return err
		}

		if division == fbs {
			if err := u.UpdateIndexJSON(json); err != nil {
				return err
			}
		}
	}

	return u.Writer.PurgeCache(context.Background())
}

func (u *Updater) UpdateAllJSON() error {
	teamMap, err := u.getTeamInfo()
	if err != nil {
		return err
	}

	if err := u.UpdateAvailRanksJSON(); err != nil {
		return err
	}

	if err := u.UpdateTeamsJSON(teamMap); err != nil {
		return err
	}

	if err := u.UpdateGameCountJSON(teamMap); err != nil {
		return err
	}

	teams := []int64{}
	if err := u.DB.Model(&database.TeamWeekResult{}).
		Distinct("team_id").Pluck("team_id", &teams).Error; err != nil {
		return err
	}
	for _, team := range teams {
		if err := u.UpdateTeamRankJSON(teamMap[team]); err != nil {
			return err
		}
	}

	yearInfo, err := u.getYearInfo()
	if err != nil {
		return err
	}

	var latestRanking *rankingsJSON

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

				weekJSON := []*resultJSON{}
				for _, week := range weekRankings {
					temp := week
					weekJSON = append(weekJSON, toJSON(&temp, teamMap))
				}
				err = u.UpdateRankJSON(&rankingsJSON{
					Division:   division,
					Year:       year.Year,
					Week:       week,
					Postseason: false,
					Results:    weekJSON,
				})
				if err != nil {
					return err
				}
			}

			var final *rankingsJSON
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

				weekJSON := []*resultJSON{}
				for _, week := range weekRankings {
					temp := week
					weekJSON = append(weekJSON, toJSON(&temp, teamMap))
				}
				final = &rankingsJSON{
					Division:   division,
					Year:       year.Year,
					Week:       1,
					Postseason: true,
					Results:    weekJSON,
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

				weekJSON := []*resultJSON{}
				for _, week := range weekRankings {
					temp := week
					weekJSON = append(weekJSON, toJSON(&temp, teamMap))
				}
				final = &rankingsJSON{
					Division:   division,
					Year:       year.Year,
					Week:       year.Weeks + 1,
					Postseason: false,
					Results:    weekJSON,
				}
			}
			if err := u.UpdateRankJSON(final); err != nil {
				return err
			}

			if division == fbs {
				latestRanking = final
			}
		}
	}

	if err := u.UpdateIndexJSON(latestRanking); err != nil {
		return err
	}

	return u.Writer.PurgeCache(context.Background())
}

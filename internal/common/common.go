package common

import (
	"log"

	"github.com/robby-barton/stats-api/internal/database"
)

func GetCurrentYear(db *database.DB) (int64, error) {
	ret, err := db.Query("select * from game order by day desc limit 1")
	if err != nil {
		return 0, err
	}
	log.Println(ret)

	return ret[0]["season"].(int64), nil
}

func GetTimeframe(db *database.DB, year int64, week int64) (retYear int64, retWeek int64, ps int64, retErr error) {
	retYear = year
	retWeek = week

	if retYear == 0 { // Year is needed no matter what
		retYear, retErr = GetCurrentYear(db)
		if retErr != nil {
			return
		}
	}

	var ret []map[string]interface{}
	if retWeek == 0 {
		ret, retErr = db.Query("select * from game where season = $1 order by day desc limit 1", retYear)
		if retErr != nil {
			return
		}
	}

	game := ret[0]
	retYear = game["season"].(int64)
	retWeek = game["week"].(int64)
	ps = game["postseason"].(int64)
	retErr = nil // make explicit

	return
}

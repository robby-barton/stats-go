package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/config"
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
)

func main() {
	cfg := config.SetupConfig()

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		panic(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	rootCmd := &cobra.Command{
		Use:   "ranker",
		Short: "College sports computer ranking calculator",
	}
	rootCmd.SilenceUsage = true

	footballCmd := sportRankCmd(db, "cfb", true)
	basketballCmd := sportRankCmd(db, "cbb", false)

	rootCmd.AddCommand(footballCmd, basketballCmd)

	rootCmd.Execute() //nolint:errcheck // cobra prints errors; exit code unused
}

func sportRankCmd(db *gorm.DB, sport string, hasFCS bool) *cobra.Command {
	var year, week int64
	var top int
	var fcs, rating bool

	use := "football"
	short := "Calculate college football rankings"
	if sport == "cbb" {
		use = "basketball"
		short = "Calculate college basketball rankings"
	}

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(_ *cobra.Command, _ []string) error {
			r := ranking.Ranker{
				DB:    db,
				Year:  year,
				Week:  week,
				Fcs:   fcs,
				Sport: sport,
			}

			start := time.Now()
			div, err := r.CalculateRanking()
			duration := time.Since(start)
			if err != nil {
				return err
			}

			// sanitize input
			if top <= 0 || top > len(div) {
				top = len(div)
			}

			if rating {
				r.PrintSRS(div, top)
			} else {
				r.PrintRankings(div, top)
			}
			fmt.Println(duration) //nolint:forbidigo // allow
			return nil
		},
	}

	cmd.Flags().Int64VarP(&year, "year", "y", 0, "ranking year")
	cmd.Flags().Int64VarP(&week, "week", "w", 0, "ranking week")
	cmd.Flags().IntVarP(&top, "top", "t", 0, "print top N teams")
	cmd.Flags().BoolVarP(&rating, "rating", "r", false, "print rating")
	if hasFCS {
		cmd.Flags().BoolVarP(&fcs, "fcs", "f", false, "rank FCS")
	}

	return cmd
}

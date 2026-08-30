package espn

// testTeamInfoResponse returns a populated TeamInfoESPN for testing.
func testTeamInfoResponse() TeamInfoESPN {
	return TeamInfoESPN{
		Sports: []TeamInfoSport{
			{
				ID:   90,
				Name: "Football",
				Slug: "football",
				Leagues: []League{
					{
						ID:           23,
						Name:         "National Collegiate Athletic Association",
						Abbreviation: "NCAAF",
						ShortName:    "NCAAF",
						Slug:         "college-football",
						Year:         2023,
						Teams: []TeamWrap{
							{Team: TeamInfo{
								ID: 1, Name: "Crimson Tide", DisplayName: "Alabama Crimson Tide",
								Abbreviation: "ALA", Location: "Alabama", Slug: "alabama",
							}},
							{Team: TeamInfo{
								ID: 2, Name: "Tigers", DisplayName: "Clemson Tigers",
								Abbreviation: "CLEM", Location: "Clemson", Slug: "clemson",
							}},
						},
					},
				},
			},
		},
	}
}

package espn

// testScheduleResponse returns a populated GameScheduleESPN for testing.
// It contains 3 games (2 final, 1 in-progress), calendar with 2 entries
// (regular season with 15 weeks, postseason), and defaults.
func testScheduleResponse() GameScheduleESPN {
	return GameScheduleESPN{
		Content: Content{
			Schedule: map[string]Day{
				"2023-09-02": {
					Games: []Game{
						{
							ID: 1001,
							Status: Status{
								StatusType: StatusType{
									Name:      "STATUS_FINAL",
									Completed: true,
								},
							},
							Competitions: []Competition{
								{
									Competitors: []Competitor{
										{ID: 1, Team: ScheduleTeam{ID: 1, ConferenceID: 100}, Score: 28, HomeAway: "home"},
										{ID: 2, Team: ScheduleTeam{ID: 2, ConferenceID: 100}, Score: 14, HomeAway: "away"},
									},
								},
							},
						},
						{
							ID: 1002,
							Status: Status{
								StatusType: StatusType{
									Name:      "STATUS_FINAL",
									Completed: true,
								},
							},
							Competitions: []Competition{
								{
									Competitors: []Competitor{
										{ID: 3, Team: ScheduleTeam{ID: 3, ConferenceID: 200}, Score: 21, HomeAway: "home"},
										{ID: 4, Team: ScheduleTeam{ID: 4, ConferenceID: 200}, Score: 10, HomeAway: "away"},
									},
								},
							},
						},
						{
							ID: 1003,
							Status: Status{
								StatusType: StatusType{
									Name:      "STATUS_IN_PROGRESS",
									Completed: false,
								},
							},
							Competitions: []Competition{
								{
									Competitors: []Competitor{
										{ID: 5, Team: ScheduleTeam{ID: 5, ConferenceID: 100}, Score: 7, HomeAway: "home"},
										{ID: 6, Team: ScheduleTeam{ID: 6, ConferenceID: 200}, Score: 3, HomeAway: "away"},
									},
								},
							},
						},
					},
				},
			},
			Parameters: Parameters{
				Week:       1,
				Year:       2023,
				SeasonType: 2,
				Group:      80,
			},
			Defaults: Parameters{
				Week:       1,
				Year:       2023,
				SeasonType: 2,
				Group:      80,
			},
			Calendar: []Calendar{
				{
					StartDate:  "2023-08-26T07:00Z",
					EndDate:    "2023-12-03T07:59Z",
					SeasonType: 2,
					Weeks: []Week{
						{Num: 0, StartDate: "2023-08-26T07:00Z", EndDate: "2023-09-04T06:59Z"},
						{Num: 1, StartDate: "2023-09-04T07:00Z", EndDate: "2023-09-11T06:59Z"},
						{Num: 2, StartDate: "2023-09-11T07:00Z", EndDate: "2023-09-18T06:59Z"},
						{Num: 3, StartDate: "2023-09-18T07:00Z", EndDate: "2023-09-25T06:59Z"},
						{Num: 4, StartDate: "2023-09-25T07:00Z", EndDate: "2023-10-02T06:59Z"},
						{Num: 5, StartDate: "2023-10-02T07:00Z", EndDate: "2023-10-09T06:59Z"},
						{Num: 6, StartDate: "2023-10-09T07:00Z", EndDate: "2023-10-16T06:59Z"},
						{Num: 7, StartDate: "2023-10-16T07:00Z", EndDate: "2023-10-23T06:59Z"},
						{Num: 8, StartDate: "2023-10-23T07:00Z", EndDate: "2023-10-30T06:59Z"},
						{Num: 9, StartDate: "2023-10-30T07:00Z", EndDate: "2023-11-06T06:59Z"},
						{Num: 10, StartDate: "2023-11-06T07:00Z", EndDate: "2023-11-13T07:59Z"},
						{Num: 11, StartDate: "2023-11-13T08:00Z", EndDate: "2023-11-20T07:59Z"},
						{Num: 12, StartDate: "2023-11-20T08:00Z", EndDate: "2023-11-27T07:59Z"},
						{Num: 13, StartDate: "2023-11-27T08:00Z", EndDate: "2023-12-04T07:59Z"},
						{Num: 14, StartDate: "2023-12-04T08:00Z", EndDate: "2023-12-11T07:59Z"},
					},
				},
				{
					StartDate:  "2023-12-16T08:00Z",
					EndDate:    "2024-01-09T07:59Z",
					SeasonType: 3,
					Weeks: []Week{
						{Num: 1, StartDate: "2023-12-16T08:00Z", EndDate: "2024-01-09T07:59Z"},
					},
				},
			},
		},
	}
}

// testGameInfoResponse returns a populated GameInfoESPN for testing.
func testGameInfoResponse() GameInfoESPN {
	return GameInfoESPN{
		GamePackage: GamePackage{
			Header: Header{
				ID: 1001,
				Competitions: []Competitions{
					{
						ID:       1001,
						Date:     "2023-09-02T23:00Z",
						ConfGame: true,
						Neutral:  false,
						Competitors: []Competitors{
							{HomeAway: "home", ID: 1, Score: 28},
							{HomeAway: "away", ID: 2, Score: 14},
						},
						Status: Status{
							StatusType: StatusType{
								Name:      "STATUS_FINAL",
								Completed: true,
							},
						},
					},
				},
				Season: Season{Year: 2023, Type: 2},
				Week:   1,
			},
			Boxscore: Boxscore{
				Teams: []Teams{
					{
						Team: Team{ID: 1},
						Statistics: []TeamStatistics{
							{Name: "firstDowns", DisplayValue: "22"},
							{Name: "totalYards", DisplayValue: "450"},
						},
					},
					{
						Team: Team{ID: 2},
						Statistics: []TeamStatistics{
							{Name: "firstDowns", DisplayValue: "15"},
							{Name: "totalYards", DisplayValue: "300"},
						},
					},
				},
				Players: []Players{
					{
						Team: Team{ID: 1},
						Statistics: []PlayerStatistics{
							{
								Name:   "passing",
								Labels: []string{"C/ATT", "YDS", "TD", "INT"},
								Totals: []string{"20/30", "285", "3", "1"},
								Athletes: []AthleteStats{
									{
										Athlete: Athlete{ID: 101, FirstName: "John", LastName: "Doe"},
										Stats:   []string{"20/30", "285", "3", "1"},
									},
								},
							},
						},
					},
					{
						Team: Team{ID: 2},
						Statistics: []PlayerStatistics{
							{
								Name:   "passing",
								Labels: []string{"C/ATT", "YDS", "TD", "INT"},
								Totals: []string{"15/25", "180", "1", "2"},
								Athletes: []AthleteStats{
									{
										Athlete: Athlete{ID: 201, FirstName: "Jane", LastName: "Smith"},
										Stats:   []string{"15/25", "180", "1", "2"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// testTeamInfoResponse returns a populated TeamInfoESPN for testing.
func testTeamInfoResponse() TeamInfoESPN {
	return TeamInfoESPN{
		Sports: []Sport{
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

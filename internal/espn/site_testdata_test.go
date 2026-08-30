package espn

// Real ESPN site.api responses captured 2026-08-29 (2026 season, week 1),
// trimmed to the fields the parser consumes. Values are unchanged from the
// live responses; only superfluous objects (links, venues, extra events,
// unused stat categories) are dropped.

// siteScoreboardFixture is a trimmed scoreboard response for
// ?groups=80 covering the three live-week statuses (final, in progress,
// scheduled) so the STATUS_FINAL filter is exercised against real data.
const siteScoreboardFixture = `{
 "leagues": [
  {
   "id": "23",
   "name": "NCAA - Football",
   "abbreviation": "NCAAF",
   "slug": "college-football",
   "season": {
    "year": 2026,
    "startDate": "2026-02-01T08:00Z",
    "endDate": "2027-01-28T07:59Z",
    "displayName": "2026",
    "type": {
     "id": "2",
     "type": 2,
     "name": "Regular Season",
     "abbreviation": "reg"
    }
   }
  }
 ],
 "season": {
  "type": 2,
  "year": 2026
 },
 "week": {
  "number": 1
 },
 "events": [
  {
   "id": "401864494",
   "date": "2026-08-29T19:00Z",
   "name": "San Jos\u00e9 State Spartans at USC Trojans",
   "shortName": "SJSU @ USC",
   "season": {
    "year": 2026,
    "type": 2,
    "slug": "regular-season"
   },
   "week": {
    "number": 1
   },
   "status": {
    "clock": 0.0,
    "displayClock": "0:00",
    "period": 4,
    "type": {
     "id": "3",
     "name": "STATUS_FINAL",
     "state": "post",
     "completed": true,
     "description": "Final",
     "detail": "Final",
     "shortDetail": "Final"
    }
   },
   "competitions": [
    {
     "id": "401864494",
     "date": "2026-08-29T19:00Z",
     "neutralSite": false,
     "conferenceCompetition": false,
     "status": {
      "clock": 0.0,
      "displayClock": "0:00",
      "period": 4,
      "type": {
       "id": "3",
       "name": "STATUS_FINAL",
       "state": "post",
       "completed": true,
       "description": "Final",
       "detail": "Final",
       "shortDetail": "Final"
      }
     },
     "competitors": [
      {
       "id": "30",
       "homeAway": "home",
       "score": "42",
       "team": {
        "id": "30",
        "conferenceId": "5"
       }
      },
      {
       "id": "23",
       "homeAway": "away",
       "score": "26",
       "team": {
        "id": "23",
        "conferenceId": "17"
       }
      }
     ]
    }
   ]
  },
  {
   "id": "401862693",
   "date": "2026-08-30T02:00Z",
   "name": "Memphis Tigers at UNLV Rebels",
   "shortName": "MEM @ UNLV",
   "season": {
    "year": 2026,
    "type": 2,
    "slug": "regular-season"
   },
   "week": {
    "number": 1
   },
   "status": {
    "clock": 531.0,
    "displayClock": "8:51",
    "period": 2,
    "type": {
     "id": "2",
     "name": "STATUS_IN_PROGRESS",
     "state": "in",
     "completed": false,
     "description": "In Progress",
     "detail": "8:51 - 2nd Quarter",
     "shortDetail": "8:51 - 2nd"
    }
   },
   "competitions": [
    {
     "id": "401862693",
     "date": "2026-08-30T02:00Z",
     "neutralSite": false,
     "conferenceCompetition": false,
     "status": {
      "clock": 531.0,
      "displayClock": "8:51",
      "period": 2,
      "type": {
       "id": "2",
       "name": "STATUS_IN_PROGRESS",
       "state": "in",
       "completed": false,
       "description": "In Progress",
       "detail": "8:51 - 2nd Quarter",
       "shortDetail": "8:51 - 2nd"
      }
     },
     "competitors": [
      {
       "id": "2439",
       "homeAway": "home",
       "score": "0",
       "team": {
        "id": "2439",
        "conferenceId": "17"
       }
      },
      {
       "id": "235",
       "homeAway": "away",
       "score": "0",
       "team": {
        "id": "235",
        "conferenceId": "151"
       }
      }
     ]
    }
   ]
  },
  {
   "id": "401858423",
   "date": "2026-09-03T22:00Z",
   "name": "Massachusetts Minutemen at Rutgers Scarlet Knights",
   "shortName": "MASS @ RUTG",
   "season": {
    "year": 2026,
    "type": 2,
    "slug": "regular-season"
   },
   "week": {
    "number": 1
   },
   "status": {
    "clock": 0.0,
    "displayClock": "0:00",
    "period": 0,
    "type": {
     "id": "1",
     "name": "STATUS_SCHEDULED",
     "state": "pre",
     "completed": false,
     "description": "Scheduled",
     "detail": "Thu, September 3rd at 6:00 PM EDT",
     "shortDetail": "9/3 - 6:00 PM EDT"
    }
   },
   "competitions": [
    {
     "id": "401858423",
     "date": "2026-09-03T22:00Z",
     "neutralSite": false,
     "conferenceCompetition": false,
     "status": {
      "clock": 0.0,
      "displayClock": "0:00",
      "period": 0,
      "type": {
       "id": "1",
       "name": "STATUS_SCHEDULED",
       "state": "pre",
       "completed": false,
       "description": "Scheduled",
       "detail": "Thu, September 3rd at 6:00 PM EDT",
       "shortDetail": "9/3 - 6:00 PM EDT"
      }
     },
     "competitors": [
      {
       "id": "164",
       "homeAway": "home",
       "score": "0",
       "team": {
        "id": "164",
        "conferenceId": "5"
       }
      },
      {
       "id": "113",
       "homeAway": "away",
       "score": "0",
       "team": {
        "id": "113",
        "conferenceId": "15"
       }
      }
     ]
    }
   ]
  }
 ]
}`

// siteSummaryFixture is a trimmed site.api summary response for event
// 401864494 (USC 42, San Jose State 26), trimmed to the fields the game
// parser consumes (header + boxscore team/player statistics).
const siteSummaryFixture = `{
 "header": {
  "id": "401864494",
  "uid": "s:20~l:23~e:401864494",
  "season": {
   "year": 2026,
   "current": true,
   "type": 2
  },
  "week": 1,
  "competitions": [
   {
    "id": "401864494",
    "date": "2026-08-29T19:00Z",
    "conferenceCompetition": false,
    "neutralSite": false,
    "status": {
     "type": {
      "id": "3",
      "name": "STATUS_FINAL",
      "state": "post",
      "completed": true,
      "description": "Final",
      "detail": "Final",
      "shortDetail": "Final"
     }
    },
    "competitors": [
     {
      "id": "30",
      "homeAway": "home",
      "score": "42",
      "winner": true,
      "team": {
       "id": "30",
       "displayName": "USC Trojans"
      }
     },
     {
      "id": "23",
      "homeAway": "away",
      "score": "26",
      "winner": false,
      "team": {
       "id": "23",
       "displayName": "San Jos\u00e9 State Spartans"
      }
     }
    ]
   }
  ]
 },
 "boxscore": {
  "teams": [
   {
    "homeAway": "away",
    "team": {
     "id": "23",
     "displayName": "San Jos\u00e9 State Spartans"
    },
    "statistics": [
     {
      "name": "firstDowns",
      "displayValue": "19",
      "value": 19.0,
      "label": "1st Downs"
     },
     {
      "name": "thirdDownEff",
      "displayValue": "5-11",
      "value": 0.4545454545,
      "label": "3rd down efficiency"
     },
     {
      "name": "fourthDownEff",
      "displayValue": "0-1",
      "value": 0,
      "label": "4th down efficiency"
     },
     {
      "name": "totalYards",
      "displayValue": "336",
      "value": "-",
      "label": "Total Yards"
     },
     {
      "name": "netPassingYards",
      "displayValue": "234",
      "value": 234.0,
      "label": "Passing"
     },
     {
      "name": "completionAttempts",
      "displayValue": "21/32",
      "value": "-",
      "label": "Comp/Att"
     },
     {
      "name": "yardsPerPass",
      "displayValue": "7.3",
      "value": 7.313,
      "label": "Yards per pass"
     },
     {
      "name": "rushingYards",
      "displayValue": "102",
      "value": 102.0,
      "label": "Rushing"
     },
     {
      "name": "rushingAttempts",
      "displayValue": "24",
      "value": 24.0,
      "label": "Rushing Attempts"
     },
     {
      "name": "yardsPerRushAttempt",
      "displayValue": "4.3",
      "value": 4.25,
      "label": "Yards per rush"
     },
     {
      "name": "totalPenaltiesYards",
      "displayValue": "1-15",
      "value": "-",
      "label": "Penalties"
     },
     {
      "name": "turnovers",
      "displayValue": "0",
      "value": "-",
      "label": "Turnovers"
     },
     {
      "name": "fumblesLost",
      "displayValue": "0",
      "value": 0.0,
      "label": "Fumbles lost"
     },
     {
      "name": "interceptions",
      "displayValue": "0",
      "value": 0.0,
      "label": "Interceptions thrown"
     },
     {
      "name": "possessionTime",
      "displayValue": "22:45",
      "value": 1365,
      "label": "Possession"
     }
    ]
   },
   {
    "homeAway": "home",
    "team": {
     "id": "30",
     "displayName": "USC Trojans"
    },
    "statistics": [
     {
      "name": "firstDowns",
      "displayValue": "29",
      "value": 29.0,
      "label": "1st Downs"
     },
     {
      "name": "thirdDownEff",
      "displayValue": "9-13",
      "value": 0.6923076923,
      "label": "3rd down efficiency"
     },
     {
      "name": "fourthDownEff",
      "displayValue": "1-2",
      "value": 0.5,
      "label": "4th down efficiency"
     },
     {
      "name": "totalYards",
      "displayValue": "505",
      "value": "-",
      "label": "Total Yards"
     },
     {
      "name": "netPassingYards",
      "displayValue": "341",
      "value": 341.0,
      "label": "Passing"
     },
     {
      "name": "completionAttempts",
      "displayValue": "30/36",
      "value": "-",
      "label": "Comp/Att"
     },
     {
      "name": "yardsPerPass",
      "displayValue": "9.5",
      "value": 9.472,
      "label": "Yards per pass"
     },
     {
      "name": "rushingYards",
      "displayValue": "164",
      "value": 164.0,
      "label": "Rushing"
     },
     {
      "name": "rushingAttempts",
      "displayValue": "40",
      "value": 40.0,
      "label": "Rushing Attempts"
     },
     {
      "name": "yardsPerRushAttempt",
      "displayValue": "4.1",
      "value": 4.1,
      "label": "Yards per rush"
     },
     {
      "name": "totalPenaltiesYards",
      "displayValue": "5-34",
      "value": "-",
      "label": "Penalties"
     },
     {
      "name": "turnovers",
      "displayValue": "1",
      "value": "-",
      "label": "Turnovers"
     },
     {
      "name": "fumblesLost",
      "displayValue": "0",
      "value": 0.0,
      "label": "Fumbles lost"
     },
     {
      "name": "interceptions",
      "displayValue": "1",
      "value": 1.0,
      "label": "Interceptions thrown"
     },
     {
      "name": "possessionTime",
      "displayValue": "37:15",
      "value": 2235,
      "label": "Possession"
     }
    ]
   }
  ],
  "players": [
   {
    "team": {
     "id": "23"
    },
    "statistics": [
     {
      "name": "passing",
      "labels": [
       "C/ATT",
       "YDS",
       "AVG",
       "TD",
       "INT"
      ],
      "totals": [
       "21/32",
       "234",
       "7.3",
       "2",
       "0"
      ],
      "athletes": [
       {
        "athlete": {
         "id": "5295238",
         "firstName": "Luke",
         "lastName": "Weaver"
        },
        "stats": [
         "21/32",
         "234",
         "7.3",
         "2",
         "0"
        ]
       }
      ]
     },
     {
      "name": "rushing",
      "labels": [
       "CAR",
       "YDS",
       "AVG",
       "TD",
       "LONG"
      ],
      "totals": [
       "24",
       "102",
       "4.3",
       "1",
       "24"
      ],
      "athletes": [
       {
        "athlete": {
         "id": "5295238",
         "firstName": "Luke",
         "lastName": "Weaver"
        },
        "stats": [
         "8",
         "42",
         "5.3",
         "1",
         "12"
        ]
       },
       {
        "athlete": {
         "id": "4685254",
         "firstName": "Jabari",
         "lastName": "Bates"
        },
        "stats": [
         "11",
         "37",
         "3.4",
         "0",
         "13"
        ]
       }
      ]
     }
    ]
   },
   {
    "team": {
     "id": "30"
    },
    "statistics": [
     {
      "name": "passing",
      "labels": [
       "C/ATT",
       "YDS",
       "AVG",
       "TD",
       "INT"
      ],
      "totals": [
       "30/36",
       "341",
       "9.5",
       "2",
       "1"
      ],
      "athletes": [
       {
        "athlete": {
         "id": "4685454",
         "firstName": "Jayden",
         "lastName": "Maiava"
        },
        "stats": [
         "25/29",
         "286",
         "9.9",
         "2",
         "0"
        ]
       },
       {
        "athlete": {
         "id": "5161854",
         "firstName": "Jonas",
         "lastName": "Williams"
        },
        "stats": [
         "5/7",
         "55",
         "7.9",
         "0",
         "1"
        ]
       }
      ]
     },
     {
      "name": "rushing",
      "labels": [
       "CAR",
       "YDS",
       "AVG",
       "TD",
       "LONG"
      ],
      "totals": [
       "40",
       "164",
       "4.1",
       "4",
       "15"
      ],
      "athletes": [
       {
        "athlete": {
         "id": "5233016",
         "firstName": "King",
         "lastName": "Miller"
        },
        "stats": [
         "15",
         "63",
         "4.2",
         "1",
         "9"
        ]
       },
       {
        "athlete": {
         "id": "5295318",
         "firstName": "Waymond",
         "lastName": "Jordan"
        },
        "stats": [
         "8",
         "48",
         "6.0",
         "0",
         "11"
        ]
       }
      ]
     }
    ]
   }
  ]
 }
}`

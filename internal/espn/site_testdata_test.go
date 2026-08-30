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

// siteScoreboardCalendarFixture is a trimmed scoreboard response for
// ?dates=2026 (captured 2026-08-29), whose leagues[0].calendar carries the
// object-shaped season-type entries the football season navigation consumes
// (the plain scoreboard response omits the calendar entirely). Entry labels,
// values and dates are unchanged; only the alternateLabel/detail fields and
// all events were dropped.
const siteScoreboardCalendarFixture = `{
 "leagues": [
  {
   "id": "23",
   "name": "NCAA - Football",
   "slug": "college-football",
   "season": {
    "year": 2026,
    "startDate": "2026-02-01T08:00Z",
    "endDate": "2027-01-28T07:59Z",
    "displayName": "2026"
   },
   "calendar": [
  {
   "label": "Regular Season",
   "value": "2",
   "startDate": "2026-08-22T07:00Z",
   "endDate": "2026-12-13T07:59Z",
   "entries": [
    {
     "label": "Week 1",
     "value": "1",
     "startDate": "2026-08-22T07:00Z",
     "endDate": "2026-09-08T06:59Z"
    },
    {
     "label": "Week 2",
     "value": "2",
     "startDate": "2026-09-08T07:00Z",
     "endDate": "2026-09-14T06:59Z"
    },
    {
     "label": "Week 3",
     "value": "3",
     "startDate": "2026-09-14T07:00Z",
     "endDate": "2026-09-21T06:59Z"
    },
    {
     "label": "Week 4",
     "value": "4",
     "startDate": "2026-09-21T07:00Z",
     "endDate": "2026-09-28T06:59Z"
    },
    {
     "label": "Week 5",
     "value": "5",
     "startDate": "2026-09-28T07:00Z",
     "endDate": "2026-10-05T06:59Z"
    },
    {
     "label": "Week 6",
     "value": "6",
     "startDate": "2026-10-05T07:00Z",
     "endDate": "2026-10-12T06:59Z"
    },
    {
     "label": "Week 7",
     "value": "7",
     "startDate": "2026-10-12T07:00Z",
     "endDate": "2026-10-19T06:59Z"
    },
    {
     "label": "Week 8",
     "value": "8",
     "startDate": "2026-10-19T07:00Z",
     "endDate": "2026-10-26T06:59Z"
    },
    {
     "label": "Week 9",
     "value": "9",
     "startDate": "2026-10-26T07:00Z",
     "endDate": "2026-11-02T07:59Z"
    },
    {
     "label": "Week 10",
     "value": "10",
     "startDate": "2026-11-02T08:00Z",
     "endDate": "2026-11-09T07:59Z"
    },
    {
     "label": "Week 11",
     "value": "11",
     "startDate": "2026-11-09T08:00Z",
     "endDate": "2026-11-16T07:59Z"
    },
    {
     "label": "Week 12",
     "value": "12",
     "startDate": "2026-11-16T08:00Z",
     "endDate": "2026-11-23T07:59Z"
    },
    {
     "label": "Week 13",
     "value": "13",
     "startDate": "2026-11-23T08:00Z",
     "endDate": "2026-11-30T07:59Z"
    },
    {
     "label": "Week 14",
     "value": "14",
     "startDate": "2026-11-30T08:00Z",
     "endDate": "2026-12-07T07:59Z"
    },
    {
     "label": "Week 15",
     "value": "15",
     "startDate": "2026-12-07T08:00Z",
     "endDate": "2026-12-13T07:59Z"
    }
   ]
  },
  {
   "label": "Postseason",
   "value": "3",
   "startDate": "2026-12-13T08:00Z",
   "endDate": "2027-01-28T07:59Z",
   "entries": [
    {
     "label": "Bowls",
     "value": "1",
     "startDate": "2026-12-13T08:00Z",
     "endDate": "2027-01-28T07:59Z"
    },
    {
     "label": "CFP",
     "value": "999",
     "startDate": "2026-12-18T08:00Z",
     "endDate": "2027-01-28T07:59Z"
    }
   ]
  },
  {
   "label": "Off Season",
   "value": "4",
   "startDate": "2027-01-28T08:00Z",
   "endDate": "2027-02-01T07:59Z",
   "entries": [
    {
     "label": "All-Star",
     "value": "1",
     "startDate": "2027-01-28T08:00Z",
     "endDate": "2027-02-01T07:59Z"
    }
   ]
  }
   ]
  }
 ],
 "week": {
  "number": 1
 },
 "events": []
}`

// siteConferencesFBSFixture is a trimmed scoreboard/conferences response
// for ?groups=80 (captured 2026-08-29): the FBS root entry plus its 11
// conferences. The logo and uid fields are dropped; values are unchanged.
const siteConferencesFBSFixture = `{
 "conferences": [
  {"groupId": "80", "name": "FBS",
   "shortName": "FBS"},
  {"groupId": "1", "name": "Atlantic Coast Conference",
   "parentGroupId": "80", "shortName": "ACC"},
  {"groupId": "151", "name": "American Conference",
   "parentGroupId": "80", "shortName": "American"},
  {"groupId": "4", "name": "Big 12 Conference",
   "parentGroupId": "80", "shortName": "Big 12"},
  {"groupId": "5", "name": "Big Ten Conference",
   "parentGroupId": "80", "shortName": "Big Ten"},
  {"groupId": "12", "name": "Conference USA",
   "parentGroupId": "80", "shortName": "CUSA"},
  {"groupId": "18", "name": "FBS Independents",
   "parentGroupId": "80", "shortName": "FBS Indep."},
  {"groupId": "15", "name": "Mid-American Conference",
   "parentGroupId": "80", "shortName": "MAC"},
  {"groupId": "17", "name": "Mountain West Conference",
   "parentGroupId": "80", "shortName": "Mountain West"},
  {"groupId": "9", "name": "Pac-12 Conference",
   "parentGroupId": "80", "shortName": "Pac-12"},
  {"groupId": "8", "name": "Southeastern Conference",
   "parentGroupId": "80", "shortName": "SEC"},
  {"groupId": "37", "name": "Sun Belt Conference",
   "parentGroupId": "80", "shortName": "Sun Belt"}
 ]
}`

// siteConferencesFCSFixture is a trimmed scoreboard/conferences response
// for ?groups=81 (captured 2026-08-29): the FCS root entry plus its 14
// conferences.
const siteConferencesFCSFixture = `{
 "conferences": [
  {"groupId": "81", "name": "FCS",
   "shortName": "FCS"},
  {"groupId": "20", "name": "Big Sky Conference",
   "parentGroupId": "81", "shortName": "Big Sky"},
  {"groupId": "48", "name": "Coastal Athletic Association",
   "parentGroupId": "81", "shortName": "CAA"},
  {"groupId": "32", "name": "FCS Independents",
   "parentGroupId": "81", "shortName": "FCS Indep."},
  {"groupId": "22", "name": "Ivy League",
   "parentGroupId": "81", "shortName": "Ivy"},
  {"groupId": "24", "name": "Mid-Eastern Athletic Conference",
   "parentGroupId": "81", "shortName": "MEAC"},
  {"groupId": "21", "name": "Missouri Valley Football Conference",
   "parentGroupId": "81", "shortName": "MVFC"},
  {"groupId": "25", "name": "Northeast Conference",
   "parentGroupId": "81", "shortName": "NEC"},
  {"groupId": "179", "name": "Ohio Valley Conference",
   "parentGroupId": "81", "shortName": "OVC"},
  {"groupId": "27", "name": "Patriot League",
   "parentGroupId": "81", "shortName": "Patriot"},
  {"groupId": "28", "name": "Pioneer Football League",
   "parentGroupId": "81", "shortName": "Pioneer"},
  {"groupId": "29", "name": "Southern Conference",
   "parentGroupId": "81", "shortName": "Southern"},
  {"groupId": "30", "name": "Southland Conference",
   "parentGroupId": "81", "shortName": "Southland"},
  {"groupId": "31", "name": "Southwestern Athletic Conference",
   "parentGroupId": "81", "shortName": "SWAC"},
  {"groupId": "177", "name": "United Athletic Conference",
   "parentGroupId": "81", "shortName": "UAC"}
 ]
}`

// siteConferencesDIIFixture is a trimmed scoreboard/conferences response
// for ?groups=57 (captured 2026-08-29): the DII root entry plus its 17
// conferences.
const siteConferencesDIIFixture = `{
 "conferences": [
  {"groupId": "57", "name": "NCAA Division II",
   "shortName": "NCAA Division II"},
  {"groupId": "104", "name": "CIAA",
   "parentGroupId": "57", "shortName": "CIAA"},
  {"groupId": "187", "name": "Conference Carolinas",
   "parentGroupId": "57", "shortName": "Conference Carolinas"},
  {"groupId": "107", "name": "GLIAC",
   "parentGroupId": "57", "shortName": "GLIAC"},
  {"groupId": "146", "name": "Great American Conference",
   "parentGroupId": "57", "shortName": "Great American"},
  {"groupId": "108", "name": "Great Lakes",
   "parentGroupId": "57", "shortName": "Great Lakes"},
  {"groupId": "165", "name": "Great Midwest Athletic Conference",
   "parentGroupId": "57", "shortName": "Great Midwest Athletic"},
  {"groupId": "110", "name": "Gulf South",
   "parentGroupId": "57", "shortName": "Gulf South"},
  {"groupId": "112", "name": "Independent DII",
   "parentGroupId": "57", "shortName": "Independent DII"},
  {"groupId": "116", "name": "Lone Star",
   "parentGroupId": "57", "shortName": "Lone Star"},
  {"groupId": "118", "name": "Mid America",
   "parentGroupId": "57", "shortName": "Mid America"},
  {"groupId": "144", "name": "Mountain East Conference",
   "parentGroupId": "57", "shortName": "Mountain East"},
  {"groupId": "127", "name": "Northeast 10",
   "parentGroupId": "57", "shortName": "Northeast 10"},
  {"groupId": "129", "name": "Northern Sun",
   "parentGroupId": "57", "shortName": "Northern Sun"},
  {"groupId": "133", "name": "Pennsylvania State Athletic Conference",
   "parentGroupId": "57", "shortName": "Pennsylvania State Athletic"},
  {"groupId": "135", "name": "Rocky Mountain",
   "parentGroupId": "57", "shortName": "Rocky Mountain"},
  {"groupId": "136", "name": "SIAC",
   "parentGroupId": "57", "shortName": "SIAC"},
  {"groupId": "139", "name": "South Atlantic",
   "parentGroupId": "57", "shortName": "South Atlantic"}
 ]
}`

// siteConferencesDIIIFixture is a trimmed scoreboard/conferences response
// for ?groups=58 (captured 2026-08-29): the DIII root entry plus its 30
// conferences.
const siteConferencesDIIIFixture = `{
 "conferences": [
  {"groupId": "58", "name": "NCAA Division III",
   "shortName": "NCAA Division III"},
  {"groupId": "114", "name": "American Rivers Conference",
   "parentGroupId": "58", "shortName": "American Rivers"},
  {"groupId": "100", "name": "American Southwest",
   "parentGroupId": "58", "shortName": "American Southwest"},
  {"groupId": "102", "name": "CCIW",
   "parentGroupId": "58", "shortName": "CCIW"},
  {"groupId": "103", "name": "Centennial",
   "parentGroupId": "58", "shortName": "Centennial"},
  {"groupId": "123", "name": "Conference of New England",
   "parentGroupId": "58", "shortName": "Conference of New England"},
  {"groupId": "106", "name": "Empire 8",
   "parentGroupId": "58", "shortName": "Empire 8"},
  {"groupId": "111", "name": "Heartland",
   "parentGroupId": "58", "shortName": "Heartland"},
  {"groupId": "113", "name": "Independent DIII",
   "parentGroupId": "58", "shortName": "Independent DIII"},
  {"groupId": "178", "name": "Landmark Conference",
   "parentGroupId": "58", "shortName": "Landmark Conference"},
  {"groupId": "115", "name": "Liberty League",
   "parentGroupId": "58", "shortName": "Liberty League"},
  {"groupId": "117", "name": "Michigan",
   "parentGroupId": "58", "shortName": "Michigan"},
  {"groupId": "119", "name": "Middle Atlantic Conference",
   "parentGroupId": "58", "shortName": "Mid Atlantic"},
  {"groupId": "120", "name": "Midwest",
   "parentGroupId": "58", "shortName": "Midwest"},
  {"groupId": "121", "name": "Minnesota",
   "parentGroupId": "58", "shortName": "Minnesota"},
  {"groupId": "160", "name": "Massachusetts State Collegiate Athletic Conference",
   "parentGroupId": "58", "shortName": "MSCAC"},
  {"groupId": "128", "name": "Northern Athletics Collegiate Conference",
   "parentGroupId": "58", "shortName": "NACC"},
  {"groupId": "122", "name": "NESCAC",
   "parentGroupId": "58", "shortName": "NESCAC"},
  {"groupId": "124", "name": "New Jersey",
   "parentGroupId": "58", "shortName": "New Jersey"},
  {"groupId": "166", "name": "New England Women's and Men's Athletic Conference",
   "parentGroupId": "58", "shortName": "NEWMAC"},
  {"groupId": "126", "name": "North Coast",
   "parentGroupId": "58", "shortName": "North Coast"},
  {"groupId": "130", "name": "Northwest",
   "parentGroupId": "58", "shortName": "Northwest"},
  {"groupId": "131", "name": "Ohio",
   "parentGroupId": "58", "shortName": "Ohio"},
  {"groupId": "132", "name": "Old Dominion",
   "parentGroupId": "58", "shortName": "Old Dominion"},
  {"groupId": "134", "name": "Presidents'",
   "parentGroupId": "58", "shortName": "Presidents'"},
  {"groupId": "138", "name": "So. Cal.",
   "parentGroupId": "58", "shortName": "So. Cal."},
  {"groupId": "147", "name": "Southern Athletic Association",
   "parentGroupId": "58", "shortName": "Southern Athletic"},
  {"groupId": "148", "name": "Southern Collegiate Athletic Conference",
   "parentGroupId": "58", "shortName": "Southern Collegiate"},
  {"groupId": "142", "name": "Upper Midwest",
   "parentGroupId": "58", "shortName": "Upper Midwest"},
  {"groupId": "143", "name": "USA South",
   "parentGroupId": "58", "shortName": "USA South"},
  {"groupId": "145", "name": "Wisconsin",
   "parentGroupId": "58", "shortName": "Wisconsin"}
 ]
}`

// siteConferencesD1Fixture is a trimmed scoreboard/conferences response for
// basketball ?groups=50 (captured 2026-08-29): the D1 root entry plus its
// 32 conferences.
const siteConferencesD1Fixture = `{
 "conferences": [
  {"groupId": "50", "name": "NCAA Division I",
   "shortName": "Division I"},
  {"groupId": "3", "name": "Atlantic 10 Conference",
   "parentGroupId": "50", "shortName": "A-10"},
  {"groupId": "2", "name": "Atlantic Coast Conference",
   "parentGroupId": "50", "shortName": "ACC"},
  {"groupId": "1", "name": "America East Conference",
   "parentGroupId": "50", "shortName": "Am. East"},
  {"groupId": "62", "name": "American Conference",
   "parentGroupId": "50", "shortName": "American"},
  {"groupId": "46", "name": "Atlantic Sun Conference",
   "parentGroupId": "50", "shortName": "Atlantic Sun"},
  {"groupId": "8", "name": "Big 12 Conference",
   "parentGroupId": "50", "shortName": "Big 12"},
  {"groupId": "4", "name": "Big East Conference",
   "parentGroupId": "50", "shortName": "Big East"},
  {"groupId": "5", "name": "Big Sky Conference",
   "parentGroupId": "50", "shortName": "Big Sky"},
  {"groupId": "6", "name": "Big South Conference",
   "parentGroupId": "50", "shortName": "Big South"},
  {"groupId": "7", "name": "Big Ten Conference",
   "parentGroupId": "50", "shortName": "Big Ten"},
  {"groupId": "9", "name": "Big West Conference",
   "parentGroupId": "50", "shortName": "Big West"},
  {"groupId": "10", "name": "Coastal Athletic Association",
   "parentGroupId": "50", "shortName": "CAA"},
  {"groupId": "11", "name": "Conference USA",
   "parentGroupId": "50", "shortName": "CUSA"},
  {"groupId": "45", "name": "Horizon League",
   "parentGroupId": "50", "shortName": "Horizon"},
  {"groupId": "12", "name": "Ivy League",
   "parentGroupId": "50", "shortName": "Ivy"},
  {"groupId": "14", "name": "Mid-American Conference",
   "parentGroupId": "50", "shortName": "MAC"},
  {"groupId": "16", "name": "Mid-Eastern Athletic Conference",
   "parentGroupId": "50", "shortName": "MEAC"},
  {"groupId": "13", "name": "Metro Conference",
   "parentGroupId": "50", "shortName": "Metro"},
  {"groupId": "44", "name": "Mountain West Conference",
   "parentGroupId": "50", "shortName": "Mountain West"},
  {"groupId": "18", "name": "Missouri Valley Conference",
   "parentGroupId": "50", "shortName": "MVC"},
  {"groupId": "19", "name": "Northeast Conference",
   "parentGroupId": "50", "shortName": "NEC"},
  {"groupId": "20", "name": "Ohio Valley Conference",
   "parentGroupId": "50", "shortName": "OVC"},
  {"groupId": "21", "name": "Pac-12 Conference",
   "parentGroupId": "50", "shortName": "Pac-12"},
  {"groupId": "22", "name": "Patriot League",
   "parentGroupId": "50", "shortName": "Patriot"},
  {"groupId": "23", "name": "Southeastern Conference",
   "parentGroupId": "50", "shortName": "SEC"},
  {"groupId": "24", "name": "Southern Conference",
   "parentGroupId": "50", "shortName": "SoCon"},
  {"groupId": "25", "name": "Southland Conference",
   "parentGroupId": "50", "shortName": "Southland"},
  {"groupId": "49", "name": "Summit League",
   "parentGroupId": "50", "shortName": "Summit"},
  {"groupId": "27", "name": "Sun Belt Conference",
   "parentGroupId": "50", "shortName": "Sun Belt"},
  {"groupId": "26", "name": "Southwestern Athletic Conference",
   "parentGroupId": "50", "shortName": "SWAC"},
  {"groupId": "30", "name": "United Athletic Conference",
   "parentGroupId": "50", "shortName": "UAC"},
  {"groupId": "29", "name": "West Coast Conference",
   "parentGroupId": "50", "shortName": "WCC"}
 ]
}`

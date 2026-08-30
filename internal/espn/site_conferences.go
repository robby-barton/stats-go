package espn

import (
	"context"
	"errors"
	"fmt"
)

// ConferencesESPN represents the site.api scoreboard/conferences response.
// It lists the conferences of one division group, selected server-side via
// the `groups` query parameter (the plain endpoint returns FBS only — FCS,
// DII and DIII need an explicit `groups` value).
type ConferencesESPN struct {
	Conferences []SiteConference `json:"conferences"`
}

// SiteConference is one conference entry. IDs arrive as JSON strings and the
// division-root entry (e.g. FBS itself) carries no parentGroupId.
type SiteConference struct {
	GroupID       FlexInt64 `json:"groupId"`
	Name          string    `json:"name"`
	ParentGroupID FlexInt64 `json:"parentGroupId"`
	ShortName     string    `json:"shortName"`
}

func (r ConferencesESPN) validate() error {
	if len(r.Conferences) == 0 {
		return errors.New("conferences response missing conferences")
	}
	return nil
}

// GetConferences fetches the conference list for a division group off the
// site.api scoreboard/conferences endpoint.
func (c *Client) GetConferences(ctx context.Context, group Group) (*ConferencesESPN, error) {
	url := c.ConferencesURL() + fmt.Sprintf("?groups=%d", group)

	var res ConferencesESPN
	if err := makeRequest(ctx, c, url, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

// conferenceShortNames reduces a conferences response to the map the
// ConferenceMap consumers need (conference ID → short name), excluding the
// division-root entry whose parentGroupId is absent.
func (r ConferencesESPN) conferenceShortNames(parent Group) map[int64]string {
	names := map[int64]string{}
	for _, conf := range r.Conferences {
		if int64(conf.ParentGroupID) == int64(parent) {
			names[int64(conf.GroupID)] = conf.ShortName
		}
	}
	return names
}

// conferenceSubGroups returns the child group IDs of a division group
// (e.g. the conferences under DII), excluding the division-root entry.
func (r ConferencesESPN) conferenceSubGroups(parent Group) []int64 {
	var groups []int64
	for _, conf := range r.Conferences {
		if int64(conf.ParentGroupID) == int64(parent) {
			groups = append(groups, int64(conf.GroupID))
		}
	}
	return groups
}

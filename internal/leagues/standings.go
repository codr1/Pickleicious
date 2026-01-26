package leagues

import (
	"context"
	"errors"
	"fmt"
	"sort"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type TeamStanding struct {
	TeamID            int64  `json:"teamId"`
	TeamName          string `json:"teamName"`
	MatchesPlayed     int    `json:"matchesPlayed"`
	Wins              int    `json:"wins"`
	Losses            int    `json:"losses"`
	PointsFor         int    `json:"pointsFor"`
	PointsAgainst     int    `json:"pointsAgainst"`
	PointDifferential int    `json:"pointDifferential"`
}

type teamStats struct {
	TeamStanding
	headToHeadWins      map[int64]int
	headToHeadPointDiff map[int64]int
}

func CalculateStandings(ctx context.Context, q *dbgen.Queries, leagueID int64) ([]TeamStanding, error) {
	if q == nil {
		return nil, errors.New("queries are required")
	}
	if leagueID <= 0 {
		return nil, errors.New("league ID is required")
	}

	rows, err := q.GetLeagueStandingsData(ctx, leagueID)
	if err != nil {
		return nil, err
	}

	teams := make(map[int64]*teamStats)
	for _, row := range rows {
		entry, ok := teams[row.TeamID]
		if !ok {
			entry = &teamStats{
				TeamStanding: TeamStanding{
					TeamID:   row.TeamID,
					TeamName: row.TeamName,
				},
				headToHeadWins:      make(map[int64]int),
				headToHeadPointDiff: make(map[int64]int),
			}
			teams[row.TeamID] = entry
		}

		if !row.MatchID.Valid {
			continue
		}
		if !row.HomeTeamID.Valid || !row.AwayTeamID.Valid || !row.HomeScore.Valid || !row.AwayScore.Valid {
			return nil, fmt.Errorf("match %d is missing scores", row.MatchID.Int64)
		}

		teamScore, opponentScore, opponentID, err := resolveMatchScore(row, entry.TeamID)
		if err != nil {
			return nil, err
		}

		entry.MatchesPlayed++
		entry.PointsFor += teamScore
		entry.PointsAgainst += opponentScore
		entry.PointDifferential = entry.PointsFor - entry.PointsAgainst

		if teamScore > opponentScore {
			entry.Wins++
			entry.headToHeadWins[opponentID]++
		} else if teamScore < opponentScore {
			entry.Losses++
		} else {
			return nil, fmt.Errorf("match %d is tied; ties are not supported", row.MatchID.Int64)
		}
		entry.headToHeadPointDiff[opponentID] += teamScore - opponentScore
	}

	ordered := make([]*teamStats, 0, len(teams))
	for _, team := range teams {
		ordered = append(ordered, team)
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Wins != ordered[j].Wins {
			return ordered[i].Wins > ordered[j].Wins
		}
		return ordered[i].TeamName < ordered[j].TeamName
	})

	sortStandingsByTiebreakers(ordered)

	standings := make([]TeamStanding, 0, len(ordered))
	for _, team := range ordered {
		standings = append(standings, team.TeamStanding)
	}
	return standings, nil
}

func resolveMatchScore(row dbgen.GetLeagueStandingsDataRow, teamID int64) (int, int, int64, error) {
	homeID := row.HomeTeamID.Int64
	awayID := row.AwayTeamID.Int64
	homeScore := int(row.HomeScore.Int64)
	awayScore := int(row.AwayScore.Int64)

	switch teamID {
	case homeID:
		return homeScore, awayScore, awayID, nil
	case awayID:
		return awayScore, homeScore, homeID, nil
	default:
		return 0, 0, 0, fmt.Errorf("match %d does not include team %d", row.MatchID.Int64, teamID)
	}
}

func sortStandingsByTiebreakers(ordered []*teamStats) {
	if len(ordered) < 2 {
		return
	}

	start := 0
	for start < len(ordered) {
		end := start + 1
		for end < len(ordered) && ordered[end].Wins == ordered[start].Wins {
			end++
		}

		if end-start > 1 {
			group := ordered[start:end]
			groupSet := make(map[int64]struct{}, len(group))
			for _, team := range group {
				groupSet[team.TeamID] = struct{}{}
			}

			sort.SliceStable(group, func(i, j int) bool {
				headToHeadWinsI := headToHeadWins(group[i], groupSet)
				headToHeadWinsJ := headToHeadWins(group[j], groupSet)
				if headToHeadWinsI != headToHeadWinsJ {
					return headToHeadWinsI > headToHeadWinsJ
				}
				if group[i].PointDifferential != group[j].PointDifferential {
					return group[i].PointDifferential > group[j].PointDifferential
				}
				headToHeadDiffI := headToHeadPointDiff(group[i], groupSet)
				headToHeadDiffJ := headToHeadPointDiff(group[j], groupSet)
				if headToHeadDiffI != headToHeadDiffJ {
					return headToHeadDiffI > headToHeadDiffJ
				}
				return group[i].TeamName < group[j].TeamName
			})
		}

		start = end
	}
}

func headToHeadWins(team *teamStats, group map[int64]struct{}) int {
	total := 0
	for opponentID, wins := range team.headToHeadWins {
		if _, ok := group[opponentID]; ok {
			total += wins
		}
	}
	return total
}

func headToHeadPointDiff(team *teamStats, group map[int64]struct{}) int {
	total := 0
	for opponentID, diff := range team.headToHeadPointDiff {
		if _, ok := group[opponentID]; ok {
			total += diff
		}
	}
	return total
}

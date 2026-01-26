package leagues

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type ScheduledMatch struct {
	LeagueID  int64
	Round     int
	HomeTeam  dbgen.LeagueTeam
	AwayTeam  dbgen.LeagueTeam
	Court     dbgen.Court
	StartTime time.Time
	EndTime   time.Time
}

type matchSlot struct {
	Start time.Time
	End   time.Time
	Court dbgen.Court
}

type dayHours struct {
	Opens  time.Time
	Closes time.Time
}

func GenerateRoundRobinSchedule(leagueID int64, teams []dbgen.LeagueTeam, startDate, endDate time.Time, courts []dbgen.Court, operatingHours []dbgen.OperatingHour, matchDuration time.Duration) ([]ScheduledMatch, error) {
	if leagueID <= 0 {
		return nil, errors.New("league ID is required")
	}
	if len(teams) < 2 {
		return nil, errors.New("at least two teams are required")
	}
	if len(courts) == 0 {
		return nil, errors.New("at least one court is required")
	}
	if matchDuration <= 0 {
		return nil, errors.New("match duration must be positive")
	}
	startDate = truncateDate(startDate)
	endDate = truncateDate(endDate)
	if endDate.Before(startDate) {
		return nil, errors.New("start date must be on or before end date")
	}

	pairs, err := buildRoundRobinPairs(teams)
	if err != nil {
		return nil, err
	}

	slots, err := buildMatchSlots(startDate, endDate, courts, operatingHours, matchDuration)
	if err != nil {
		return nil, err
	}
	if len(slots) < len(pairs) {
		return nil, fmt.Errorf("insufficient slots: need %d matches but only %d available", len(pairs), len(slots))
	}

	schedule := make([]ScheduledMatch, 0, len(pairs))
	for idx, pairing := range pairs {
		slot := slots[idx]
		schedule = append(schedule, ScheduledMatch{
			LeagueID:  leagueID,
			Round:     pairing.Round,
			HomeTeam:  pairing.HomeTeam,
			AwayTeam:  pairing.AwayTeam,
			Court:     slot.Court,
			StartTime: slot.Start,
			EndTime:   slot.End,
		})
	}
	return schedule, nil
}

type roundPair struct {
	Round    int
	HomeTeam dbgen.LeagueTeam
	AwayTeam dbgen.LeagueTeam
}

func buildRoundRobinPairs(teams []dbgen.LeagueTeam) ([]roundPair, error) {
	working := make([]*dbgen.LeagueTeam, 0, len(teams)+1)
	for i := range teams {
		working = append(working, &teams[i])
	}
	if len(working)%2 == 1 {
		working = append(working, nil)
	}

	rounds := len(working) - 1
	pairs := make([]roundPair, 0, rounds*len(working)/2)

	for round := 0; round < rounds; round++ {
		for i := 0; i < len(working)/2; i++ {
			left := working[i]
			right := working[len(working)-1-i]
			if left == nil || right == nil {
				continue
			}
			home := *left
			away := *right
			if i == 0 && round%2 == 1 {
				home, away = away, home
			}
			pairs = append(pairs, roundPair{
				Round:    round + 1,
				HomeTeam: home,
				AwayTeam: away,
			})
		}
		rotateTeams(working)
	}

	return pairs, nil
}

func rotateTeams(teams []*dbgen.LeagueTeam) {
	if len(teams) <= 2 {
		return
	}
	last := teams[len(teams)-1]
	copy(teams[2:], teams[1:len(teams)-1])
	teams[1] = last
}

func buildMatchSlots(startDate, endDate time.Time, courts []dbgen.Court, operatingHours []dbgen.OperatingHour, matchDuration time.Duration) ([]matchSlot, error) {
	hoursByDay, err := buildHoursByDay(operatingHours)
	if err != nil {
		return nil, err
	}
	if len(hoursByDay) == 0 {
		return nil, errors.New("operating hours are required")
	}

	var slots []matchSlot
	for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
		hours, ok := hoursByDay[int(date.Weekday())]
		if !ok {
			continue
		}
		dayOpen := time.Date(date.Year(), date.Month(), date.Day(), hours.Opens.Hour(), hours.Opens.Minute(), 0, 0, date.Location())
		dayClose := time.Date(date.Year(), date.Month(), date.Day(), hours.Closes.Hour(), hours.Closes.Minute(), 0, 0, date.Location())
		if !dayClose.After(dayOpen) {
			continue
		}
		for start := dayOpen; !start.Add(matchDuration).After(dayClose); start = start.Add(matchDuration) {
			end := start.Add(matchDuration)
			for _, court := range courts {
				slots = append(slots, matchSlot{Start: start, End: end, Court: court})
			}
		}
	}

	if len(slots) == 0 {
		return nil, errors.New("no available match slots in the league date range")
	}
	return slots, nil
}

func buildHoursByDay(operatingHours []dbgen.OperatingHour) (map[int]dayHours, error) {
	result := make(map[int]dayHours)
	for _, hour := range operatingHours {
		opensRaw := apiutil.FormatOperatingHourValue(hour.OpensAt)
		closesRaw := apiutil.FormatOperatingHourValue(hour.ClosesAt)
		if strings.TrimSpace(opensRaw) == "" || strings.TrimSpace(closesRaw) == "" {
			continue
		}
		opens, err := parseTimeOfDay(opensRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid opens_at for day %d: %w", hour.DayOfWeek, err)
		}
		closes, err := parseTimeOfDay(closesRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid closes_at for day %d: %w", hour.DayOfWeek, err)
		}
		result[int(hour.DayOfWeek)] = dayHours{Opens: opens, Closes: closes}
	}
	return result, nil
}

func parseTimeOfDay(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("time is required")
	}
	parsed, err := time.Parse("15:04", raw)
	if err != nil {
		formats := []string{"3:04 PM", "03:04 PM", "3:04PM", "03:04PM"}
		for _, format := range formats {
			if parsed, err = time.Parse(format, strings.ToUpper(raw)); err == nil {
				return parsed, nil
			}
		}
		return time.Time{}, errors.New("time must be in HH:MM or H:MM AM/PM format")
	}
	return parsed, nil
}

func truncateDate(value time.Time) time.Time {
	loc := value.Location()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, loc)
}

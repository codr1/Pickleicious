package openplay

import (
	"strings"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type OpenPlayRule struct {
	dbgen.OpenPlayRule
}

// Template-facing wrapper for adding view helpers without changing dbgen.
func NewOpenPlayRule(row dbgen.OpenPlayRule) OpenPlayRule {
	return OpenPlayRule{OpenPlayRule: row}
}

func NewOpenPlayRules(rows []dbgen.OpenPlayRule) []OpenPlayRule {
	rules := make([]OpenPlayRule, len(rows))
	for i, row := range rows {
		rules[i] = NewOpenPlayRule(row)
	}
	return rules
}

type OpenPlayParticipant struct {
	dbgen.ListOpenPlayParticipantsRow
}

func NewOpenPlayParticipant(row dbgen.ListOpenPlayParticipantsRow) OpenPlayParticipant {
	return OpenPlayParticipant{ListOpenPlayParticipantsRow: row}
}

func NewOpenPlayParticipants(rows []dbgen.ListOpenPlayParticipantsRow) []OpenPlayParticipant {
	participants := make([]OpenPlayParticipant, len(rows))
	for i, row := range rows {
		participants[i] = NewOpenPlayParticipant(row)
	}
	return participants
}

func (p OpenPlayParticipant) HasPhoto() bool {
	return p.PhotoUrl.Valid && strings.TrimSpace(p.PhotoUrl.String) != ""
}

func (p OpenPlayParticipant) PhotoURL() string {
	if p.HasPhoto() {
		return strings.TrimSpace(p.PhotoUrl.String)
	}
	return ""
}

func (p OpenPlayParticipant) Initials() string {
	first := strings.TrimSpace(p.FirstName)
	last := strings.TrimSpace(p.LastName)
	if first == "" || last == "" {
		return ""
	}
	return firstInitial(first) + firstInitial(last)
}

func firstInitial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return ""
}

const (
	defaultMinParticipants           int64 = 4
	defaultMaxParticipantsPerCourt   int64 = 8
	defaultCancellationCutoffMinutes int64 = 60
	defaultMinCourts                 int64 = 1
	defaultMaxCourts                 int64 = 2
)

func (r OpenPlayRule) MinParticipantsValue() int64 {
	if r.ID == 0 && r.MinParticipants == 0 {
		return defaultMinParticipants
	}
	return r.MinParticipants
}

func (r OpenPlayRule) MaxParticipantsPerCourtValue() int64 {
	if r.ID == 0 && r.MaxParticipantsPerCourt == 0 {
		return defaultMaxParticipantsPerCourt
	}
	return r.MaxParticipantsPerCourt
}

func (r OpenPlayRule) CancellationCutoffMinutesValue() int64 {
	if r.ID == 0 && r.CancellationCutoffMinutes == 0 {
		return defaultCancellationCutoffMinutes
	}
	return r.CancellationCutoffMinutes
}

func (r OpenPlayRule) MinCourtsValue() int64 {
	if r.ID == 0 && r.MinCourts == 0 {
		return defaultMinCourts
	}
	return r.MinCourts
}

func (r OpenPlayRule) MaxCourtsValue() int64 {
	if r.ID == 0 && r.MaxCourts == 0 {
		return defaultMaxCourts
	}
	return r.MaxCourts
}

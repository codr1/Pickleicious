package reservations

import (
	"fmt"
	"strings"
	"time"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type BookingFormData struct {
	FacilityID                int64
	StartTime                 time.Time
	EndTime                   time.Time
	Courts                    []CourtOption
	ReservationTypes          []ReservationTypeOption
	Members                   []MemberOption
	SelectedCourtID           int64
	SelectedReservationTypeID int64
	PrimaryUserID             *int64
	IsOpenEvent               bool
	TeamsPerCourt             *int64
	PeoplePerTeam             *int64
	IsEdit                    bool
	ReservationID             int64
}

type CourtOption struct {
	ID    int64
	Label string
}

type ReservationTypeOption struct {
	ID   int64
	Name string
}

type MemberOption struct {
	ID    int64
	Label string
}

func NewCourtOptions(rows []dbgen.Court) []CourtOption {
	options := make([]CourtOption, 0, len(rows))
	for _, court := range rows {
		label := strings.TrimSpace(court.Name)
		if label == "" {
			label = fmt.Sprintf("Court %d", court.CourtNumber)
		} else {
			label = fmt.Sprintf("%s (Court %d)", label, court.CourtNumber)
		}
		options = append(options, CourtOption{ID: court.ID, Label: label})
	}
	return options
}

func NewReservationTypeOptions(rows []dbgen.ReservationType) []ReservationTypeOption {
	options := make([]ReservationTypeOption, 0, len(rows))
	for _, resType := range rows {
		options = append(options, ReservationTypeOption{ID: resType.ID, Name: resType.Name})
	}
	return options
}

func NewMemberOptions(rows []dbgen.ListMembersRow) []MemberOption {
	options := make([]MemberOption, 0, len(rows))
	for _, member := range rows {
		label := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
		if member.Email.Valid {
			label = fmt.Sprintf("%s - %s", label, member.Email.String)
		}
		options = append(options, MemberOption{ID: member.ID, Label: label})
	}
	return options
}

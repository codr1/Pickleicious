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

type EventBookingFormData struct {
	FacilityID                int64
	StartTime                 time.Time
	EndTime                   time.Time
	Courts                    []CourtOption
	ReservationTypes          []ReservationTypeOption
	Members                   []MemberOption
	Participants              []MemberOption
	SelectedCourtIDs          []int64
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

type FacilityOption struct {
	ID   int64
	Name string
}

type ProOption struct {
	ID   int64
	Name string
}

type StaffLessonBookingFormData struct {
	Facilities           []FacilityOption
	Pros                 []ProOption
	Members              []MemberOption
	SelectedFacilityID   int64
	SelectedProID        int64
	DateValue            string
	ShowFacilitySelector bool
}

type StaffLessonSlotOption struct {
	StartTime string
	EndTime   string
	Label     string
}

type StaffLessonSlotsData struct {
	FacilityID    int64
	ProID         int64
	ProName       string
	DateValue     string
	PrimaryUserID *int64
	Slots         []StaffLessonSlotOption
}

// NewCourtOptions converts database court rows into a slice of CourtOption.
// Each returned option contains the court ID and a human-readable label: the
// trimmed court name followed by " (Court N)" when a name is present, or
// "Court N" when the name is empty.
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

// NewMemberOptions converts database member rows into a slice of MemberOption.
// Each MemberOption.Label is the member's first and last name (trimmed); if the
// member has an email, the label appends " - <email>".
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

// NewFacilityOptions converts database facility rows into a slice of FacilityOption.
// Each input row produces a FacilityOption with the same ID and Name, preserving input order.
func NewFacilityOptions(rows []dbgen.Facility) []FacilityOption {
	options := make([]FacilityOption, 0, len(rows))
	for _, facility := range rows {
		options = append(options, FacilityOption{ID: facility.ID, Name: facility.Name})
	}
	return options
}

// NewProOptions builds ProOption entries from database pro rows.
// The Name field combines the pro's first and last name (trimmed); if an email is present it appends " - <email>". It preserves the input order.
func NewProOptions(rows []dbgen.ListProsByFacilityRow) []ProOption {
	options := make([]ProOption, 0, len(rows))
	for _, pro := range rows {
		label := strings.TrimSpace(strings.Join([]string{pro.FirstName, pro.LastName}, " "))
		if pro.Email.Valid {
			label = fmt.Sprintf("%s - %s", label, pro.Email.String)
		}
		options = append(options, ProOption{ID: pro.ID, Name: label})
	}
	return options
}

// defaultStaffLessonEndTimeValue returns the EndTime of the first slot, or an empty string if the provided slice has no slots.
func defaultStaffLessonEndTimeValue(slots []StaffLessonSlotOption) string {
	if len(slots) == 0 {
		return ""
	}
	return slots[0].EndTime
}
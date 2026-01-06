package member

import (
	"fmt"
	"strings"
	"time"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type PortalProfile struct {
	ID              int64
	FirstName       string
	LastName        string
	Email           string
	MembershipLevel int64
	HasPhoto        bool
}

func (p PortalProfile) FullName() string {
	return strings.TrimSpace(strings.TrimSpace(p.FirstName) + " " + strings.TrimSpace(p.LastName))
}

func (p PortalProfile) PhotoURL() string {
	if p.HasPhoto {
		return fmt.Sprintf("/api/v1/members/photo/%d", p.ID)
	}
	return ""
}

func (p PortalProfile) Initials() string {
	first := strings.TrimSpace(p.FirstName)
	last := strings.TrimSpace(p.LastName)
	if first == "" && last == "" {
		return "?"
	}
	initials := ""
	if first != "" {
		initials += strings.ToUpper(first[:1])
	}
	if last != "" {
		initials += strings.ToUpper(last[:1])
	}
	return initials
}

func (p PortalProfile) MembershipLabel() string {
	switch p.MembershipLevel {
	case 0:
		return "Unverified Guest"
	case 1:
		return "Verified Guest"
	case 2:
		return "Member"
	default:
		return "Member+"
	}
}

type ReservationSummary struct {
	ID                  int64
	FacilityID          int64
	FacilityName        string
	CourtName           string
	ReservationTypeName string
	StartTime           time.Time
	EndTime             time.Time
	IsOpenEvent         bool
	OtherParticipants   []string
}

type ReservationFacility struct {
	ID   int64
	Name string
}

type ReservationListData struct {
	Upcoming           []ReservationSummary
	Past               []ReservationSummary
	Facilities         []ReservationFacility
	SelectedFacilityID int64
	ShowFacilityFilter bool
}

type ReservationWidgetData struct {
	Upcoming []ReservationSummary
	Count    int
}

func NewReservationSummaries(rows []dbgen.ListReservationsByUserIDRow) []ReservationSummary {
	summaries := make([]ReservationSummary, len(rows))
	for i, row := range rows {
		var reservationTypeName string
		if row.ReservationTypeName.Valid {
			reservationTypeName = row.ReservationTypeName.String
		}
		courtName := row.CourtName
		summaries[i] = ReservationSummary{
			ID:                  row.ID,
			FacilityID:          row.FacilityID,
			FacilityName:        row.FacilityName,
			CourtName:           courtName,
			ReservationTypeName: reservationTypeName,
			StartTime:           row.StartTime,
			EndTime:             row.EndTime,
			IsOpenEvent:         row.IsOpenEvent,
		}
	}
	return summaries
}

func NewReservationWidgetData(upcoming []ReservationSummary) ReservationWidgetData {
	return ReservationWidgetData{
		Upcoming: upcoming,
		Count:    len(upcoming),
	}
}

func (r ReservationWidgetData) BadgeLabel() string {
	if r.Count > 9 {
		return "9+"
	}
	return fmt.Sprintf("%d", r.Count)
}

func (r ReservationSummary) ReservationTypeLabel() string {
	if r.ReservationTypeName != "" {
		return r.ReservationTypeName
	}
	if r.IsOpenEvent {
		return "Open Play"
	}
	return "Reservation"
}

func (r ReservationSummary) CourtLabel() string {
	if r.CourtName != "" {
		return r.CourtName
	}
	return "TBD"
}

func (r ReservationSummary) OtherParticipantsLabel() string {
	if len(r.OtherParticipants) == 0 {
		return "No other participants"
	}
	return strings.Join(r.OtherParticipants, ", ")
}

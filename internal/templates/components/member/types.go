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
	ID           int64
	FacilityName string
	StartTime    time.Time
	EndTime      time.Time
	IsOpenEvent  bool
}

func NewReservationSummaries(rows []dbgen.ListReservationsByUserIDRow) []ReservationSummary {
	summaries := make([]ReservationSummary, len(rows))
	for i, row := range rows {
		summaries[i] = ReservationSummary{
			ID:           row.ID,
			FacilityName: row.FacilityName,
			StartTime:    row.StartTime,
			EndTime:      row.EndTime,
			IsOpenEvent:  row.IsOpenEvent,
		}
	}
	return summaries
}

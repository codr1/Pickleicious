package members

import (
	"fmt"
	"strings"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type Member struct {
	dbgen.ListMembersRow
}

// NewMember creates a Member from ListMembersRow
func NewMember(row dbgen.ListMembersRow) Member {
	return Member{ListMembersRow: row}
}

// NewMembers converts a slice of ListMembersRow to Members
func NewMembers(rows []dbgen.ListMembersRow) []Member {
	members := make([]Member, len(rows))
	for i, row := range rows {
		members[i] = NewMember(row)
	}
	return members
}

// Helper methods remain the same
func (m Member) HasPhoto() bool {
	return m.PhotoID.Valid
}

func (m Member) PhotoUrl() string {
	if m.PhotoID.Valid {
		return fmt.Sprintf("/api/v1/members/photo/%d", m.ID)
	}
	return ""
}

func (m Member) EmailStr() string {
	return m.Email.String
}

func (m Member) PhoneStr() string {
	return m.Phone.String
}

func (m Member) AddressStr() string {
	return m.StreetAddress.String
}

func (m Member) CityStr() string {
	return m.City.String
}

func (m Member) StateStr() string {
	return m.State.String
}

func (m Member) PostalCodeStr() string {
	return m.PostalCode.String
}

type VisitHistory struct {
	dbgen.FacilityVisit
	FacilityName string
}

func NewVisitHistory(visits []dbgen.FacilityVisit, facilityNames map[int64]string) []VisitHistory {
	items := make([]VisitHistory, len(visits))
	for i, visit := range visits {
		name := strings.TrimSpace(facilityNames[visit.FacilityID])
		items[i] = VisitHistory{
			FacilityVisit: visit,
			FacilityName:  name,
		}
	}
	return items
}

func (v VisitHistory) FacilityLabel() string {
	if v.FacilityName != "" {
		return v.FacilityName
	}
	return fmt.Sprintf("Facility #%d", v.FacilityID)
}

func (v VisitHistory) CheckInTimeLabel() string {
	return v.CheckInTime.Format("Jan 2, 2006 3:04 PM")
}

func (v VisitHistory) ActivityLabel() string {
	activity := strings.TrimSpace(v.ActivityType.String)
	if !v.ActivityType.Valid || activity == "" {
		return "Facility Visit"
	}

	switch activity {
	case "court_reservation":
		return "Court Reservation"
	case "open_play":
		return "Open Play"
	case "league":
		return "League"
	default:
		return activity
	}
}

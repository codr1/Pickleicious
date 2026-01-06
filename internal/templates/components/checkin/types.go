package checkin

import (
	"database/sql"
	"fmt"
	"strings"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type CheckinMember struct {
	dbgen.ListMembersRow
}

func NewCheckinMember(row dbgen.ListMembersRow) CheckinMember {
	return CheckinMember{ListMembersRow: row}
}

func NewCheckinMembers(rows []dbgen.ListMembersRow) []CheckinMember {
	members := make([]CheckinMember, len(rows))
	for i, row := range rows {
		members[i] = NewCheckinMember(row)
	}
	return members
}

func NewCheckinMemberFromMember(member dbgen.GetMemberByIDRow) CheckinMember {
	return CheckinMember{ListMembersRow: dbgen.ListMembersRow{
		ID:              member.ID,
		FirstName:       member.FirstName,
		LastName:        member.LastName,
		Email:           member.Email,
		WaiverSigned:    member.WaiverSigned,
		MembershipLevel: member.MembershipLevel,
		PhotoID:         member.PhotoID,
		PhotoUrl:        sql.NullString{},
	}}
}

func (m CheckinMember) DisplayName() string {
	name := strings.TrimSpace(strings.Join([]string{m.FirstName, m.LastName}, " "))
	if name == "" {
		return fmt.Sprintf("Member #%d", m.ID)
	}
	return name
}

func (m CheckinMember) HasPhoto() bool {
	return m.PhotoID.Valid || strings.TrimSpace(m.PhotoUrl.String) != ""
}

func (m CheckinMember) PhotoURL() string {
	photoURL := strings.TrimSpace(m.PhotoUrl.String)
	if m.PhotoUrl.Valid && photoURL != "" {
		return photoURL
	}
	if m.PhotoID.Valid {
		return fmt.Sprintf("/api/v1/members/photo/%d", m.ID)
	}
	return ""
}

func (m CheckinMember) Initials() string {
	first := strings.TrimSpace(m.FirstName)
	last := strings.TrimSpace(m.LastName)
	if first == "" || last == "" {
		return ""
	}
	return firstInitial(first) + firstInitial(last)
}

func (m CheckinMember) EmailStr() string {
	if !m.Email.Valid {
		return ""
	}
	return strings.TrimSpace(m.Email.String)
}

func (m CheckinMember) MembershipLabel() string {
	switch m.MembershipLevel {
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

type FacilityVisit struct {
	dbgen.FacilityVisit
	MemberFirstName string
	MemberLastName  string
	MemberPhotoURL  string
}

func NewFacilityVisit(row dbgen.FacilityVisit) FacilityVisit {
	return FacilityVisit{FacilityVisit: row}
}

func NewFacilityVisitWithMember(visit dbgen.FacilityVisit, member dbgen.GetMemberByIDRow) FacilityVisit {
	return FacilityVisit{
		FacilityVisit:   visit,
		MemberFirstName: member.FirstName,
		MemberLastName:  member.LastName,
		MemberPhotoURL:  memberPhotoURL(member.ID, member.PhotoID),
	}
}

func NewFacilityVisits(rows []dbgen.FacilityVisit) []FacilityVisit {
	visits := make([]FacilityVisit, len(rows))
	for i, row := range rows {
		visits[i] = NewFacilityVisit(row)
	}
	return visits
}

func (v FacilityVisit) MemberName() string {
	name := strings.TrimSpace(strings.Join([]string{v.MemberFirstName, v.MemberLastName}, " "))
	if name == "" {
		return fmt.Sprintf("Member #%d", v.UserID)
	}
	return name
}

func (v FacilityVisit) MemberInitials() string {
	first := strings.TrimSpace(v.MemberFirstName)
	last := strings.TrimSpace(v.MemberLastName)
	if first == "" || last == "" {
		return ""
	}
	return firstInitial(first) + firstInitial(last)
}

func (v FacilityVisit) MemberPhoto() string {
	return strings.TrimSpace(v.MemberPhotoURL)
}

func (v FacilityVisit) CheckInTimeLabel() string {
	return v.CheckInTime.Format("3:04 PM")
}

func (v FacilityVisit) ActivityLabel() string {
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

func firstInitial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return ""
}

func memberPhotoURL(memberID int64, photoID sql.NullInt64) string {
	if photoID.Valid {
		return fmt.Sprintf("/api/v1/members/photo/%d", memberID)
	}
	return ""
}

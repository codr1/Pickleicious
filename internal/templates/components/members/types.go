package members

import (
	"fmt"

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

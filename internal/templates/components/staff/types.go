package staff

import (
	"fmt"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type Staff struct {
	dbgen.ListStaffRow
}

type ProScheduleViewData struct {
	ProName  string
	Sessions []dbgen.GetFutureProSessionsByStaffIDRow
}

type NotificationDetailData struct {
	Notification dbgen.StaffNotification
	MemberName   string
	MemberEmail  string
	MemberPhone  string
	LessonDate   string
	LessonTime   string
	CourtLabel   string
}

type WaitlistConfigFormData struct {
	FacilityID                int64
	MaxWaitlistSize           int64
	NotificationMode          string
	OfferExpiryMinutes        int64
	NotificationWindowMinutes int64
}

const defaultWaitlistOfferExpiryMinutes int64 = 30

// NewStaff creates a Staff from ListStaffRow.
func NewStaff(row dbgen.ListStaffRow) Staff {
	return Staff{ListStaffRow: row}
}

// NewStaffList converts a slice of ListStaffRow to Staff entries.
func NewStaffList(rows []dbgen.ListStaffRow) []Staff {
	staff := make([]Staff, len(rows))
	for i, row := range rows {
		staff[i] = NewStaff(row)
	}
	return staff
}

func (s Staff) EmailStr() string {
	if s.Email.Valid {
		return s.Email.String
	}
	return ""
}

func (s Staff) PhoneStr() string {
	if s.Phone.Valid {
		return s.Phone.String
	}
	return ""
}

func (s Staff) RoleStr() string {
	return s.Role
}

func (s Staff) FacilityStr() string {
	if s.HomeFacilityID.Valid {
		return fmt.Sprintf("%d", s.HomeFacilityID.Int64)
	}
	return ""
}

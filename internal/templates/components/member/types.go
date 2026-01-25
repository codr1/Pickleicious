package member

import (
	"fmt"
	"strings"
	"time"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/templates/components/reservations"
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
	ReservationTypeID   int64
	ProFirstName        string
	ProLastName         string
	StartTime           time.Time
	EndTime             time.Time
	IsOpenEvent         bool
	OtherParticipants   []string
	RefundPercentage    int64
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

type OpenPlaySessionSummary struct {
	ID               int64
	RuleName         string
	StartTime        time.Time
	EndTime          time.Time
	Status           string
	ParticipantCount int64
	MinParticipants  int64
	IsSignedUp       bool
}

type OpenPlayListData struct {
	Upcoming []OpenPlaySessionSummary
}

type CancellationPenaltyData struct {
	ReservationID    int64     `json:"reservation_id"`
	FeePercentage    int64     `json:"fee_percentage"`
	RefundPercentage int64     `json:"refund_percentage"`
	HoursBeforeStart int64     `json:"hours_before_start"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	CourtName        string    `json:"court_name"`
	FacilityName     string    `json:"facility_name"`
	ExpiresAt        time.Time `json:"expires_at"`
	CalculatedAt     time.Time `json:"penalty_calculated_at"`
}

type MemberBookingSlot struct {
	StartTime time.Time
	EndTime   time.Time
	Label     string
}

type LessonProCard struct {
	ID       int64
	Name     string
	Initials string
}

type LessonSlotOption struct {
	StartTime string
	EndTime   string
	Label     string
}

type DatePickerData struct {
	Year  int
	Month int
	Day   int
}

func (d DatePickerData) YearOptions() []int {
	now := time.Now()
	return []int{now.Year(), now.Year() + 1}
}

func (d DatePickerData) MonthOptions() []int {
	months := make([]int, 12)
	for i := 1; i <= 12; i++ {
		months[i-1] = i
	}
	return months
}

func (d DatePickerData) DayOptions() []int {
	now := time.Now()
	year := d.Year
	month := d.Month
	if year <= 0 || month < 1 || month > 12 {
		year = now.Year()
		month = int(now.Month())
	}
	daysInMonth := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, now.Location()).Day()
	days := make([]int, daysInMonth)
	for i := 1; i <= daysInMonth; i++ {
		days[i-1] = i
	}
	return days
}

type MemberBookingFormData struct {
	FacilityID            int64
	Courts                []reservations.CourtOption
	AvailableSlots        []MemberBookingSlot
	DatePicker            DatePickerData
	MaxAdvanceBookingDays int64
	WaitlistStartTime     time.Time
	WaitlistEndTime       time.Time
}

type LessonBookingFormData struct {
	Pros                  []LessonProCard
	Slots                 []LessonSlotOption
	DatePicker            DatePickerData
	SelectedProID         int64
	MaxAdvanceBookingDays int64
}

func NewReservationSummaries(rows []dbgen.ListReservationsByUserIDRow) []ReservationSummary {
	summaries := make([]ReservationSummary, len(rows))
	for i, row := range rows {
		var reservationTypeName string
		if row.ReservationTypeName.Valid {
			reservationTypeName = row.ReservationTypeName.String
		}
		var proFirstName string
		if row.ProFirstName.Valid {
			proFirstName = row.ProFirstName.String
		}
		var proLastName string
		if row.ProLastName.Valid {
			proLastName = row.ProLastName.String
		}
		courtName := row.CourtName
		summaries[i] = ReservationSummary{
			ID:                  row.ID,
			FacilityID:          row.FacilityID,
			FacilityName:        row.FacilityName,
			CourtName:           courtName,
			ReservationTypeName: reservationTypeName,
			ReservationTypeID:   row.ReservationTypeID,
			ProFirstName:        proFirstName,
			ProLastName:         proLastName,
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

func NewOpenPlaySessionSummary(row dbgen.ListMemberUpcomingOpenPlaySessionsRow) OpenPlaySessionSummary {
	return OpenPlaySessionSummary{
		ID:               row.ID,
		RuleName:         row.RuleName,
		StartTime:        row.StartTime,
		EndTime:          row.EndTime,
		Status:           row.Status,
		ParticipantCount: row.ParticipantCount,
		MinParticipants:  row.MinParticipants,
	}
}

func NewOpenPlaySessionSummaries(rows []dbgen.ListMemberUpcomingOpenPlaySessionsRow) []OpenPlaySessionSummary {
	summaries := make([]OpenPlaySessionSummary, len(rows))
	for i, row := range rows {
		summaries[i] = NewOpenPlaySessionSummary(row)
	}
	return summaries
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

func (r ReservationSummary) IsProSession() bool {
	return strings.EqualFold(r.ReservationTypeName, "PRO_SESSION")
}

func (r ReservationSummary) ProName() string {
	name := strings.TrimSpace(strings.TrimSpace(r.ProFirstName) + " " + strings.TrimSpace(r.ProLastName))
	if name == "" {
		return "TBD"
	}
	return name
}

func defaultLessonEndTimeValue(slots []LessonSlotOption) string {
	if len(slots) == 0 {
		return ""
	}
	return slots[0].EndTime
}

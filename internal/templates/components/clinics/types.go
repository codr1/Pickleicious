package clinics

import (
	"fmt"
	"strings"
	"time"
)

type EnrollmentStatus string

const (
	EnrollmentStatusNone       EnrollmentStatus = ""
	EnrollmentStatusEnrolled   EnrollmentStatus = "enrolled"
	EnrollmentStatusWaitlisted EnrollmentStatus = "waitlisted"
	EnrollmentStatusCancelled  EnrollmentStatus = "cancelled"
)

type ClinicListData struct {
	Upcoming []ClinicSessionCard
}

type ClinicSessionCard struct {
	ID              int64
	Name            string
	Description     string
	ProFirstName    string
	ProLastName     string
	StartTime       time.Time
	EndTime         time.Time
	MinParticipants int64
	MaxParticipants int64
	EnrolledCount   int64
	WaitlistCount   int64
	PriceCents      int64
	IsFull          bool
	UserStatus      EnrollmentStatus
}

func (c ClinicSessionCard) ProName() string {
	name := strings.TrimSpace(strings.Join([]string{c.ProFirstName, c.ProLastName}, " "))
	if name == "" {
		return "Staff"
	}
	return name
}

func (c ClinicSessionCard) EnrollmentSummary() string {
	return fmt.Sprintf("%d/%d enrolled", c.EnrolledCount, c.MaxParticipants)
}

func (c ClinicSessionCard) WaitlistSummary() string {
	if c.WaitlistCount <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", c.WaitlistCount)
}

func (c ClinicSessionCard) PriceDisplay() string {
	return fmt.Sprintf("$%.2f", float64(c.PriceCents)/100)
}

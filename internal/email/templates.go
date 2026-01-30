package email

import (
	"fmt"
	"strings"
	"time"
)

type ConfirmationEmail struct {
	Subject string
	Body    string
}

type ConfirmationDetails struct {
	FacilityName       string
	Date               string
	TimeRange          string
	Courts             string
	CancellationPolicy string
}

type CancellationDetails struct {
	FacilityName     string
	ReservationType  string
	Date             string
	TimeRange        string
	Courts           string
	Reason           string
	RefundPercentage *int64
	FeeWaived        bool
}

type ReminderDetails struct {
	FacilityName    string
	ReservationType string
	Date            string
	TimeRange       string
	Courts          string
}

func FormatDateTimeRange(start, end time.Time) (string, string) {
	date := start.Format("Monday, Jan 2, 2006")
	timeRange := fmt.Sprintf("%s - %s %s", start.Format("3:04 PM"), end.Format("3:04 PM"), start.Format("MST"))
	return date, timeRange
}

func BuildGameConfirmation(details ConfirmationDetails) ConfirmationEmail {
	return buildConfirmationEmail("Game", "Game Reservation Confirmed", details)
}

func BuildProSessionConfirmation(details ConfirmationDetails) ConfirmationEmail {
	return buildConfirmationEmail("Pro Session", "Pro Session Confirmed", details)
}

func BuildOpenPlayConfirmation(details ConfirmationDetails) ConfirmationEmail {
	return buildConfirmationEmail("Open Play", "Open Play Signup Confirmed", details)
}

func ReservationTypeLabel(reservationType string) string {
	switch strings.TrimSpace(reservationType) {
	case "OPEN_PLAY":
		return "Open Play"
	case "PRO_SESSION":
		return "Pro Session"
	case "GAME":
		return "Court Reservation"
	}
	return "Reservation"
}

func BuildCancellationEmail(details CancellationDetails) ConfirmationEmail {
	facilityName := strings.TrimSpace(details.FacilityName)
	if facilityName == "" {
		facilityName = "your facility"
	}
	reservationType := strings.TrimSpace(details.ReservationType)
	if reservationType == "" {
		reservationType = "Reservation"
	}
	date := strings.TrimSpace(details.Date)
	if date == "" {
		date = "TBD"
	}
	timeRange := strings.TrimSpace(details.TimeRange)
	if timeRange == "" {
		timeRange = "TBD"
	}
	courts := strings.TrimSpace(details.Courts)
	if courts == "" {
		courts = "TBD"
	}

	subject := fmt.Sprintf("%s Cancelled", reservationType)
	if facilityName != "" {
		subject = fmt.Sprintf("%s - %s", subject, facilityName)
	}

	lines := []string{
		fmt.Sprintf("Your %s booking has been cancelled.", reservationType),
		"",
		fmt.Sprintf("Facility: %s", facilityName),
		fmt.Sprintf("Reservation type: %s", reservationType),
		fmt.Sprintf("Date: %s", date),
		fmt.Sprintf("Time: %s", timeRange),
		fmt.Sprintf("Courts: %s", courts),
	}

	reason := strings.TrimSpace(details.Reason)
	if reason != "" {
		lines = append(lines, fmt.Sprintf("Reason: %s", reason))
	}

	if details.FeeWaived {
		lines = append(lines, "Fee waived: Yes")
	} else if details.RefundPercentage != nil {
		lines = append(lines, fmt.Sprintf("Refund: %d%%", *details.RefundPercentage))
	}

	return ConfirmationEmail{
		Subject: subject,
		Body:    strings.Join(lines, "\n"),
	}
}

func BuildReminderEmail(details ReminderDetails) ConfirmationEmail {
	facilityName := strings.TrimSpace(details.FacilityName)
	if facilityName == "" {
		facilityName = "your facility"
	}
	reservationType := strings.TrimSpace(details.ReservationType)
	if reservationType == "" {
		reservationType = "Reservation"
	}
	date := strings.TrimSpace(details.Date)
	if date == "" {
		date = "TBD"
	}
	timeRange := strings.TrimSpace(details.TimeRange)
	if timeRange == "" {
		timeRange = "TBD"
	}
	courts := strings.TrimSpace(details.Courts)
	if courts == "" {
		courts = "TBD"
	}

	subject := fmt.Sprintf("Upcoming %s Reminder", reservationType)
	if facilityName != "" {
		subject = fmt.Sprintf("%s - %s", subject, facilityName)
	}

	lines := []string{
		fmt.Sprintf("Reminder: your %s booking is coming up.", reservationType),
		"",
		fmt.Sprintf("Facility: %s", facilityName),
		fmt.Sprintf("Reservation type: %s", reservationType),
		fmt.Sprintf("Date: %s", date),
		fmt.Sprintf("Time: %s", timeRange),
		fmt.Sprintf("Courts: %s", courts),
	}

	return ConfirmationEmail{
		Subject: subject,
		Body:    strings.Join(lines, "\n"),
	}
}

func buildConfirmationEmail(reservationType, subjectPrefix string, details ConfirmationDetails) ConfirmationEmail {
	facilityName := strings.TrimSpace(details.FacilityName)
	if facilityName == "" {
		facilityName = "your facility"
	}
	courts := strings.TrimSpace(details.Courts)
	if courts == "" {
		courts = "TBD"
	}
	cancellationPolicy := strings.TrimSpace(details.CancellationPolicy)
	if cancellationPolicy == "" {
		cancellationPolicy = "Contact the facility for cancellation policy details."
	}

	subject := subjectPrefix
	if facilityName != "" {
		subject = fmt.Sprintf("%s - %s", subjectPrefix, facilityName)
	}

	lines := []string{
		fmt.Sprintf("Your %s booking is confirmed.", reservationType),
		"",
		fmt.Sprintf("Facility: %s", facilityName),
		fmt.Sprintf("Reservation type: %s", reservationType),
		fmt.Sprintf("Date: %s", strings.TrimSpace(details.Date)),
		fmt.Sprintf("Time: %s", strings.TrimSpace(details.TimeRange)),
		fmt.Sprintf("Courts: %s", courts),
		fmt.Sprintf("Cancellation policy: %s", cancellationPolicy),
	}

	return ConfirmationEmail{
		Subject: subject,
		Body:    strings.Join(lines, "\n"),
	}
}

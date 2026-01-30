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
	Date              string
	TimeRange          string
	Courts             string
	CancellationPolicy string
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

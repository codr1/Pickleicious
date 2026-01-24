package waitlist

import (
	"strings"
	"time"
)

type WaitlistEntry struct {
	ID           int64
	FacilityID   int64
	FacilityName string
	CourtName    string
	StartTime    time.Time
	EndTime      time.Time
	Position     int64
	Status       string
}

func (e WaitlistEntry) CourtLabel() string {
	label := strings.TrimSpace(e.CourtName)
	if label == "" {
		return "Any court"
	}
	return label
}

func (e WaitlistEntry) StatusLabel() string {
	status := strings.ToLower(strings.TrimSpace(e.Status))
	switch status {
	case "pending":
		return "Pending"
	case "notified":
		return "Notified"
	case "expired":
		return "Expired"
	case "fulfilled":
		return "Fulfilled"
	default:
		if status == "" {
			return "Pending"
		}
		return strings.ToUpper(status[:1]) + status[1:]
	}
}

type WaitlistEntryListData struct {
	Entries []WaitlistEntry
}

type WaitlistJoinButtonData struct {
	FacilityID int64
	CourtID    *int64
	StartTime  time.Time
	EndTime    time.Time
	Disabled   bool
	Label      string
}

func (d WaitlistJoinButtonData) ButtonLabel() string {
	if strings.TrimSpace(d.Label) == "" {
		return "Join waitlist"
	}
	return d.Label
}

package waitlist

import (
	"strconv"
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

func (d WaitlistJoinButtonData) HXVals() string {
	var builder strings.Builder
	builder.WriteString(`{"facility_id":`)
	builder.WriteString(strconv.FormatInt(d.FacilityID, 10))
	if d.CourtID != nil {
		builder.WriteString(`,"court_id":`)
		builder.WriteString(strconv.FormatInt(*d.CourtID, 10))
	}
	builder.WriteString(`,"start_time":"`)
	builder.WriteString(d.StartTime.Format("2006-01-02T15:04"))
	builder.WriteString(`","end_time":"`)
	builder.WriteString(d.EndTime.Format("2006-01-02T15:04"))
	builder.WriteString(`"}`)
	return builder.String()
}

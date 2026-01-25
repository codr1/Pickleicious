package notifications

import (
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type Notification struct {
	dbgen.StaffNotification
}

// NewNotification creates a view wrapper for StaffNotification.
func NewNotification(row dbgen.StaffNotification) Notification {
	return Notification{StaffNotification: row}
}

// NewNotifications converts staff notifications into view wrappers.
func NewNotifications(rows []dbgen.StaffNotification) []Notification {
	notifications := make([]Notification, len(rows))
	for i, row := range rows {
		notifications[i] = NewNotification(row)
	}
	return notifications
}

func (n Notification) TypeLabel() string {
	switch n.NotificationType {
	case "scale_up":
		return "Scale up"
	case "scale_down":
		return "Scale down"
	case "cancelled":
		return "Cancelled"
	case "lesson_cancelled":
		return "Lesson Cancelled"
	default:
		return n.NotificationType
	}
}

func (n Notification) BadgeClass() string {
	switch n.NotificationType {
	case "scale_up":
		return "bg-blue-100 text-blue-800"
	case "scale_down":
		return "bg-yellow-100 text-yellow-800"
	case "cancelled":
		return "bg-red-100 text-red-800"
	case "lesson_cancelled":
		return "bg-orange-100 text-orange-800"
	default:
		return "bg-muted text-muted-foreground"
	}
}

func (n Notification) Timestamp() string {
	return n.CreatedAt.Format("2006-01-02 15:04")
}

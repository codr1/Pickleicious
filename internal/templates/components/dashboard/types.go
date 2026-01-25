package dashboard

type CancellationMetrics struct {
	Count                 int64
	TotalReservations     int64
	Rate                  float64
	TotalRefundPercentage float64
}

type BookingTypeCount struct {
	TypeID   int64
	TypeName string
	Count    int64
}

type DashboardData struct {
	FacilityID          int64
	FacilityName        string
	DateRange           string
	UtilizationRate     float64
	ScheduledCount      int64
	BookingsByType      []BookingTypeCount
	CancellationMetrics CancellationMetrics
	CheckinCount        int64
	Granularity         string
}

package operatinghours

type DayHours struct {
	DayOfWeek int64
	OpensAt   string
	ClosesAt  string
	IsClosed  bool
}

type BookingConfigData struct {
	FacilityID            int64
	MaxAdvanceBookingDays int64
	MaxMemberReservations int64
}

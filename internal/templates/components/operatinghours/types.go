package operatinghours

type DayHours struct {
	DayOfWeek int64
	OpensAt   string
	ClosesAt  string
	IsClosed  bool
}

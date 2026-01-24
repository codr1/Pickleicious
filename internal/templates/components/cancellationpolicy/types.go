package cancellationpolicy

type PolicyTierData struct {
	ID                  int64
	MinHoursBefore      int64
	RefundPercentage    int64
	ReservationTypeID   *int64
	ReservationTypeName *string
}

type ReservationTypeOption struct {
	ID   int64
	Name string
}

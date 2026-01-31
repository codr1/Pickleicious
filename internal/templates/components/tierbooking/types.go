package tierbooking

type TierBookingWindowData struct {
	MembershipLevel int64
	Label           string
	MaxAdvanceDays  int64
	Note            string
}

type TierBookingPageData struct {
	FacilityID                    int64
	TierBookingEnabled            bool
	FacilityDefaultMaxAdvanceDays int64
	Windows                       []TierBookingWindowData
}

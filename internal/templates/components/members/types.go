package members

type Member struct {
	ID            int
	FirstName     string
	LastName      string
	Email         string
	Phone         string
	PhotoUrl      string
	Status        string
	HasPhoto      bool
	StreetAddress string
	City          string
	State         string
	PostalCode    string
	DateOfBirth   string // Store as YYYY-MM-DD format
	WaiverSigned  bool
	CardLastFour  string
	CardType      string
}

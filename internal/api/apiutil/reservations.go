package apiutil

import (
	"fmt"
	"strings"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

func ReservationCourtLabel(courts []dbgen.ListReservationCourtsRow) string {
	if len(courts) == 0 {
		return "TBD"
	}
	labels := make([]string, len(courts))
	for i, court := range courts {
		labels[i] = fmt.Sprintf("Court %d", court.CourtNumber)
	}
	return strings.Join(labels, ", ")
}

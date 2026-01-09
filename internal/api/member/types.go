package member

import (
	membertempl "github.com/codr1/Pickleicious/internal/templates/components/member"
)

type cancellationPenaltyError struct {
	Penalty membertempl.CancellationPenaltyData
}

func (e cancellationPenaltyError) Error() string {
	return "cancellation confirmation required"
}

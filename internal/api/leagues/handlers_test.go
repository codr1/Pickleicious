package leagues

import (
	"database/sql"
	"testing"
	"time"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

func TestRosterLockedAtTimezoneBehindUTC(t *testing.T) {
	loc := time.FixedZone("UTC-7", -7*60*60)
	league := dbgen.League{
		RosterLockDate: sql.NullTime{
			Time:  time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC),
			Valid: true,
		},
	}

	beforeLocalMidnight := time.Date(2024, 5, 10, 6, 59, 0, 0, time.UTC)
	if rosterLockedAt(league, loc, beforeLocalMidnight) {
		t.Fatalf("expected roster unlocked before local midnight")
	}

	atLocalMidnight := time.Date(2024, 5, 10, 7, 0, 0, 0, time.UTC)
	if !rosterLockedAt(league, loc, atLocalMidnight) {
		t.Fatalf("expected roster locked at local midnight")
	}
}

func TestRosterLockedAtTimezoneAheadUTC(t *testing.T) {
	loc := time.FixedZone("UTC+9", 9*60*60)
	league := dbgen.League{
		RosterLockDate: sql.NullTime{
			Time:  time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC),
			Valid: true,
		},
	}

	beforeLocalMidnight := time.Date(2024, 5, 9, 14, 59, 0, 0, time.UTC)
	if rosterLockedAt(league, loc, beforeLocalMidnight) {
		t.Fatalf("expected roster unlocked before local midnight")
	}

	atLocalMidnight := time.Date(2024, 5, 9, 15, 0, 0, 0, time.UTC)
	if !rosterLockedAt(league, loc, atLocalMidnight) {
		t.Fatalf("expected roster locked at local midnight")
	}
}

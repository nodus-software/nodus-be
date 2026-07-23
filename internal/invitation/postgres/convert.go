package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func fromTimestamptz(t pgtype.Timestamptz) time.Time {
	return t.Time
}

func fromNullTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tm := t.Time
	return &tm
}

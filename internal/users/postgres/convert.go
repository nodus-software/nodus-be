package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func fromNullTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tm := t.Time
	return &tm
}

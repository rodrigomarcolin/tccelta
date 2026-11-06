package postgres

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/port"
)

type tripSeeder struct{ q dbgen.Querier }

// TripSeeder cria viagens de teste em gold.trip sobre q (papel dono das tabelas: tccelta_api só insere pela API2).
func TripSeeder(q dbgen.Querier) port.TripSeeder { return &tripSeeder{q} }

func (r *tripSeeder) SeedBootTrip(ctx context.Context, s port.SeedTrip) (uuid.UUID, error) {
	gen, boot := s.BootGeneration, s.BootID
	return r.q.SeedTripBoot(ctx, dbgen.SeedTripBootParams{TripCategory: s.Category, DriverName: s.Driver,
		DriverNameNormalized: strings.ToLower(strings.TrimSpace(s.Driver)), Fuel: s.Fuel, DongleID: s.DongleID,
		BootGeneration: &gen, BootID: &boot, TargetSeq: s.TargetSeq})
}

func (r *tripSeeder) SeedIntervalTrip(ctx context.Context, s port.SeedTrip) (uuid.UUID, error) {
	start, end := s.Start, s.End
	return r.q.SeedTripInterval(ctx, dbgen.SeedTripIntervalParams{TripCategory: s.Category, DriverName: s.Driver,
		DriverNameNormalized: strings.ToLower(strings.TrimSpace(s.Driver)), Fuel: s.Fuel, DongleID: s.DongleID,
		IntervalStart: &start, IntervalEnd: &end, TargetSeq: s.TargetSeq})
}

var _ port.TripSeeder = (*tripSeeder)(nil)

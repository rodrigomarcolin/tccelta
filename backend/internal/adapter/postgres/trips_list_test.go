package postgres

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/trip"
)

func TestInsertTranslatesTripRepeated(t *testing.T) {
	q := &fakeQ{}
	r := &trips{q: q}
	for _, name := range []string{"trip_boot_unique", "trip_interval_unique"} {
		q.insert = &pgconn.PgError{Code: "23505", ConstraintName: name}
		_, err := r.Insert(context.Background(), trip.Draft{})
		if !errors.Is(err, trip.ErrTripRepeated) || errors.Is(err, trip.ErrDriverRepeated) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// outro código com o mesmo nome de índice não é a violação
	q.insert = &pgconn.PgError{Code: "23503", ConstraintName: "trip_boot_unique"}
	if _, err := r.Insert(context.Background(), trip.Draft{}); errors.Is(err, trip.ErrTripRepeated) {
		t.Fatalf("%v", err)
	}
}

// listQ responde as consultas da listagem e guarda os argumentos.
type listQ struct {
	dbgen.Querier
	rows      []dbgen.ListTripsRow
	routes    []dbgen.ListTripRoutesRow
	same      dbgen.GoldTrip
	sameErr   error
	sameArgs  dbgen.FindSameTripParams
	countArg  string
	listAt    time.Time
	listID    uuid.UUID
	listArg   string
	listLimit int32
	routeIDs  []uuid.UUID
}

func (q *listQ) FindSameTrip(_ context.Context, a dbgen.FindSameTripParams) (dbgen.GoldTrip, error) {
	q.sameArgs = a
	return q.same, q.sameErr
}

func (q *listQ) CountTrips(_ context.Context, search string) (int64, error) {
	q.countArg = search
	return 134, nil
}

func (q *listQ) ListTrips(_ context.Context, at time.Time, id uuid.UUID, search string, limit int32) ([]dbgen.ListTripsRow, error) {
	q.listAt, q.listID, q.listArg, q.listLimit = at, id, search, limit
	return q.rows, nil
}

func (q *listQ) ListTripRoutes(_ context.Context, ids []uuid.UUID) ([]dbgen.ListTripRoutesRow, error) {
	q.routeIDs = ids
	return q.routes, nil
}

func TestFindSame(t *testing.T) {
	gen, boot := int32(2), int32(8)
	q := &listQ{sameErr: pgx.ErrNoRows}
	r := &trips{q: q}
	if _, found, err := r.FindSame(context.Background(), trip.Draft{DongleID: 1, EntryType: trip.EntryTypeBoot, BootGeneration: &gen, BootID: &boot}); err != nil || found {
		t.Fatalf("%v %v", err, found)
	}
	if a := q.sameArgs; a.DongleID != 1 || a.EntryType != "boot" || *a.BootGeneration != 2 || *a.BootID != 8 || a.IntervalStart != nil {
		t.Fatalf("%+v", a)
	}
	id := uuid.New()
	q.sameErr, q.same = nil, dbgen.GoldTrip{TripID: id, DriverName: "João", Status: "ready"}
	got, found, err := r.FindSame(context.Background(), trip.Draft{})
	if err != nil || !found || got.ID != id || got.DriverName != "João" {
		t.Fatalf("%v %v %+v", err, found, got)
	}
	boom := errors.New("boom")
	q.sameErr = boom
	if _, _, err := r.FindSame(context.Background(), trip.Draft{}); !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
}

func listRow(status string, at time.Time) dbgen.ListTripsRow {
	return dbgen.ListTripsRow{TripID: uuid.New(), Status: status, DriverName: "x", CreatedAt: at, EntryType: trip.EntryTypeBoot}
}

func TestReadListPaging(t *testing.T) {
	base := time.Date(2026, 10, 28, 17, 3, 0, 123456000, time.UTC)
	// limite 2: a consulta pede 3 linhas; a terceira só diz que há próxima e não entra na página nem nas rotas
	rows := []dbgen.ListTripsRow{listRow("ready", base), listRow("pending", base.Add(-time.Minute)), listRow("ready", base.Add(-2*time.Minute))}
	dist := math.NaN()
	rows[0].DistanceKm, rows[0].DurationS = &dist, ptr(600.0)
	route := []byte(`{"t_ms":[0,1000],"lat":[-23.5,-23.6],"lon":[-46.6,-46.7],"segmentos":{"velocidade_kmh":[10],"faixa":["v1"]}}`)
	q := &listQ{rows: rows, routes: []dbgen.ListTripRoutesRow{{TripID: rows[0].TripID, Route: route}}}
	p, err := readList(context.Background(), q, trip.ListQuery{Limit: 2, Search: `a_b%c\d`})
	if err != nil {
		t.Fatal(err)
	}
	if q.listLimit != 3 || q.countArg != `a\_b\%c\\d` || q.listArg != q.countArg {
		t.Fatalf("limite %d, busca %q / %q", q.listLimit, q.countArg, q.listArg)
	}
	if !q.listAt.After(time.Date(9000, 1, 1, 0, 0, 0, 0, time.UTC)) || q.listID.String() != "ffffffff-ffff-ffff-ffff-ffffffffffff" {
		t.Fatalf("sem cursor: %v %v", q.listAt, q.listID)
	}
	if len(p.Items) != 2 || p.Total != 134 || p.Next == nil || p.Next.ID != rows[1].TripID || !p.Next.CreatedAt.Equal(rows[1].CreatedAt) {
		t.Fatalf("%+v", p)
	}
	if len(q.routeIDs) != 1 || q.routeIDs[0] != rows[0].TripID {
		t.Fatalf("só a pronta da página busca a rota: %v", q.routeIDs)
	}
	if p.Items[0].DistanceKm != nil || *p.Items[0].DurationS != 600 {
		t.Fatalf("NaN sai como nulo: %+v", p.Items[0])
	}
	if len(p.Items[0].Route) != 1 || p.Items[1].Route != nil {
		t.Fatalf("%+v", p.Items)
	}
}

func TestReadListCursorAndEmpty(t *testing.T) {
	c := trip.ListCursor{CreatedAt: time.Date(2026, 10, 28, 17, 3, 0, 123456000, time.UTC), ID: uuid.New()}
	q := &listQ{}
	p, err := readList(context.Background(), q, trip.ListQuery{Limit: 24, Cursor: &c})
	if err != nil || p.Items == nil || len(p.Items) != 0 || p.Next != nil {
		t.Fatalf("%v %+v", err, p)
	}
	if !q.listAt.Equal(c.CreatedAt) || q.listID != c.ID || q.routeIDs != nil {
		t.Fatalf("%v %v %v", q.listAt, q.listID, q.routeIDs)
	}
}

func TestReadListBadRouteJSONIsNull(t *testing.T) {
	r := listRow("ready", time.Now())
	q := &listQ{rows: []dbgen.ListTripsRow{r}, routes: []dbgen.ListTripRoutesRow{{TripID: r.TripID, Route: []byte(`lixo`)}}}
	p, err := readList(context.Background(), q, trip.ListQuery{Limit: 5})
	if err != nil || len(p.Items) != 1 || p.Items[0].Route != nil {
		t.Fatalf("%v %+v", err, p)
	}
}

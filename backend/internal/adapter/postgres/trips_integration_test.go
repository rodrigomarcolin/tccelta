//go:build integration

package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/database"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
)

// Estes testes precisam de um banco migrado e dos papéis do projeto. Apontam para ele por duas variáveis com a URL completa
// (com senha): TEST_BANCO_DONO_URL (tccelta_owner) e TEST_BANCO_API_URL (tccelta_api). Sem elas, são ignorados. Com a pilha
// de teste de pé (make e2e sem o down), rodam dentro da rede dela; ver backend/README.md.

func connect(t *testing.T, env string) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(env)
	if url == "" {
		t.Skipf("%s não definida", env)
	}
	p, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// newDongle cadastra um dongle com o papel dono e devolve o dongle_id.
func newDongle(t *testing.T, owner *pgxpool.Pool) int32 {
	t.Helper()
	dev := make([]byte, 8)
	_, _ = rand.Read(dev)
	var id int32
	err := owner.QueryRow(context.Background(),
		`INSERT INTO dongle (dev_id, kms_auth_key, kms_rec_key) VALUES ($1, 'k', 'k') RETURNING dongle_id`, dev).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func draft(dongle int32, category, name string) trip.Draft {
	boot := int32(8)
	return trip.Draft{Category: category, DriverName: name, DriverNameNormalized: trip.NormalizeName(name), Fuel: "gasolina_e30",
		DongleID: dongle, EntryType: trip.EntryTypeBoot, BootID: &boot, TargetSeq: 5}
}

func apiTx(ctx context.Context, pool *pgxpool.Pool, fn func(r port.Repositories) error) error {
	return database.InTransaction(ctx, pool, database.DefaultOptions, func(ctx context.Context, q *dbgen.Queries, _ pgx.Tx) error {
		return fn(Repositories(q))
	})
}

func TestNotifyArrivesOnlyAfterCommit(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	listener, err := pgx.Connect(ctx, os.Getenv("TEST_BANCO_API_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close(ctx)
	if _, err := listener.Exec(ctx, "LISTEN trip_pending"); err != nil {
		t.Fatal(err)
	}
	cat := "notify-" + time.Now().Format("150405.000000")

	var created trip.Trip
	rollback := errors.New("desfaz")
	err = apiTx(ctx, api, func(r port.Repositories) error {
		tr, err := r.Trips.Insert(ctx, draft(d, cat, "Rollback"))
		if err != nil {
			return err
		}
		if err := r.Trips.NotifyTrip(ctx, tr.ID); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
	if n, err := listener.WaitForNotification(wctx); err == nil {
		t.Fatalf("NOTIFY chegou apesar do rollback: %+v", n)
	}
	cancel()

	err = apiTx(ctx, api, func(r port.Repositories) error {
		tr, err := r.Trips.Insert(ctx, draft(d, cat, "Commit"))
		if err != nil {
			return err
		}
		created = tr
		return r.Trips.NotifyTrip(ctx, tr.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel = context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	n, err := listener.WaitForNotification(wctx)
	if err != nil || n.Payload != created.ID.String() {
		t.Fatalf("notificação %+v err=%v, esperava %s", n, err, created.ID)
	}
	if created.Status != trip.StatusPending || created.TargetSeq != 5 {
		t.Fatalf("%+v", created)
	}
}

func TestConcurrentSameDriverOneWins(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	cat := "dup-" + time.Now().Format("150405.000000")
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, name := range []string{"  JOÃO ", "joão"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = apiTx(ctx, api, func(r port.Repositories) error {
				_, err := r.Trips.Insert(ctx, draft(d, cat, name))
				return err
			})
		}()
	}
	wg.Wait()
	ok, repeated := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, trip.ErrDriverRepeated):
			repeated++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || repeated != 1 {
		t.Fatalf("ok=%d repetidos=%d, esperava 1 e 1", ok, repeated)
	}
	// com a viagem em failed o nome é liberado
	if _, err := owner.Exec(ctx, "UPDATE gold.trip SET status = 'failed' WHERE trip_category = $1", cat); err != nil {
		t.Fatal(err)
	}
	if err := apiTx(ctx, api, func(r port.Repositories) error {
		_, err := r.Trips.Insert(ctx, draft(d, cat, "JOÃO"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestApiRoleCannotChangeGold(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	var tr trip.Trip
	if err := apiTx(ctx, api, func(r port.Repositories) (err error) {
		tr, err = r.Trips.Insert(ctx, draft(d, "perm-"+time.Now().Format("150405.000000"), "Ana"))
		return
	}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"UPDATE gold.trip SET status = 'ready' WHERE trip_id = $1",
		"DELETE FROM gold.trip WHERE trip_id = $1",
		"DELETE FROM gold.trip_summary WHERE trip_id = $1",
		"INSERT INTO gold.trip_part (trip_id, part_no, boot_generation, boot_id, data_start_t_ms, data_end_t_ms) VALUES ($1, 1, 0, 1, 0, 1)",
	} {
		if _, err := api.Exec(ctx, sql, tr.ID); err == nil {
			t.Errorf("tccelta_api não deveria executar: %s", sql)
		}
	}
	if _, err := api.Exec(ctx, "SELECT 1 FROM gold.trip_summary LIMIT 1"); err != nil {
		t.Errorf("SELECT nas tabelas da gold deve funcionar: %v", err)
	}
}

func TestAnalysisIgnoresRecalcInProgress(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	var tr trip.Trip
	if err := apiTx(ctx, api, func(r port.Repositories) (err error) {
		tr, err = r.Trips.Insert(ctx, draft(d, "rec-"+time.Now().Format("150405.000000"), "Rec"))
		return
	}); err != nil {
		t.Fatal(err)
	}
	save := func(q dbgen.Querier, distance float64) {
		t.Helper()
		_ = q.DeleteTripSummary(ctx, tr.ID)
		_ = q.DeleteTripSeries(ctx, tr.ID)
		_ = q.DeleteTripEvents(ctx, tr.ID)
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		must(q.InsertTripSummary(ctx, dbgen.InsertTripSummaryParams{TripID: tr.ID, Warnings: []byte(`[]`), Parameters: []byte(`{}`),
			PidPeriodsMs: []byte(`{}`), DistanceKm: &distance}))
		must(q.InsertTripSeries(ctx, dbgen.InsertTripSeriesParams{TripID: tr.ID, StepMs: 100, ExposureStepMs: 100, Series: []byte(`{}`),
			SlowSeries: []byte(`{}`), Route: []byte(`{}`), RpmDistribution: []byte(`[]`), PedalDistribution: []byte(`[]`), RpmPedalMatrix: []byte(`{}`)}))
		for i := int32(1); i <= 3; i++ {
			must(q.InsertTripEvent(ctx, dbgen.InsertTripEventParams{TripID: tr.ID, EventNo: i, Type: "frenagem_brusca", PeakUnit: "m/s2"}))
		}
		must(q.SaveTripResult(ctx, dbgen.SaveTripResultParams{TripID: tr.ID}))
	}
	save(dbgen.New(owner), 0.30000000000000004)

	tx, err := owner.Begin(ctx) // recálculo em andamento: regrava tudo, sem commit
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	_ = q.DeleteTripEvents(ctx, tr.ID)
	save(q, 9.5)

	a, err := TripsWithPool(api).Analysis(ctx, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Summary.DistanceKm == nil || *a.Summary.DistanceKm != 0.30000000000000004 || len(a.Events) != 3 {
		t.Fatalf("a leitura deve ver o resultado anterior inteiro: %+v", a.Summary.DistanceKm)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	a, err = TripsWithPool(api).Analysis(ctx, tr.ID)
	if err != nil || *a.Summary.DistanceKm != 9.5 {
		t.Fatalf("depois do commit vale o novo: %v %v", err, a.Summary.DistanceKm)
	}
}

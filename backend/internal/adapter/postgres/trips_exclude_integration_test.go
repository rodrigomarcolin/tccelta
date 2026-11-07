//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
)

// Estes testes cobrem a exclusão lógica da viagem (migration 00012, EXC1) e usam as mesmas variáveis TEST_BANCO_* dos demais.

func exclude(ctx context.Context, api *pgxpool.Pool, id uuid.UUID) error {
	return apiTx(ctx, api, func(r port.Repositories) error { return r.Trips.Exclude(ctx, id) })
}

func TestExcludeHidesTripAndFreesNameAndEntry(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	u := uniq()
	cat := "exc-" + u
	t0 := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	boot, err := insertTrip(ctx, api, bootDraft(d, cat, "Ana "+u, 0, 8))
	if err != nil {
		t.Fatal(err)
	}
	interval, err := insertTrip(ctx, api, intervalDraft(d, cat, "Beto "+u, t0, t0.Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	trips := TripsWithPool(api)
	search := trip.ListQuery{Search: trip.NormalizeName(u), Limit: 10}
	if p, err := trips.List(ctx, search); err != nil || p.Total != 2 || len(p.Items) != 2 {
		t.Fatalf("%v %+v", err, p)
	}

	if err := exclude(ctx, api, boot.ID); err != nil {
		t.Fatal(err)
	}
	if err := exclude(ctx, api, interval.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{boot.ID, interval.ID} {
		if _, err := trips.Find(ctx, id); !errors.Is(err, trip.ErrNotFound) {
			t.Errorf("Find: %v", err)
		}
		if _, err := trips.Analysis(ctx, id); !errors.Is(err, trip.ErrNotFound) {
			t.Errorf("Analysis: %v", err)
		}
		// repetir a exclusão
		if err := exclude(ctx, api, id); !errors.Is(err, trip.ErrNotFound) {
			t.Errorf("segunda exclusão: %v", err)
		}
	}
	if err := exclude(ctx, api, uuid.New()); !errors.Is(err, trip.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	if p, err := trips.List(ctx, search); err != nil || p.Total != 0 || len(p.Items) != 0 {
		t.Errorf("List traz excluídas ou o total não caiu: %v %+v", err, p)
	}
	var visible int
	if err := owner.QueryRow(ctx, "SELECT count(*) FROM gold.trip WHERE trip_category = $1 AND excluded_at IS NULL", cat).Scan(&visible); err != nil || visible != 0 {
		t.Errorf("sobraram %d viagens vivas da categoria: %v", visible, err)
	}

	// o nome, o boot e o intervalo voltam a ser aceitos
	if _, found, err := findSame(ctx, api, bootDraft(d, cat, "Qualquer", 0, 8)); err != nil || found {
		t.Errorf("FindSame do boot: %v %v", found, err)
	}
	if _, found, err := findSame(ctx, api, intervalDraft(d, cat, "Qualquer", t0, t0.Add(time.Hour))); err != nil || found {
		t.Errorf("FindSame do intervalo: %v %v", found, err)
	}
	for _, x := range []trip.Draft{bootDraft(d, cat, "Ana "+u, 0, 8), intervalDraft(d, cat, "Beto "+u, t0, t0.Add(time.Hour))} {
		if _, err := insertTrip(ctx, api, x); err != nil {
			t.Errorf("recriar %s: %v", x.DriverName, err)
		}
	}
}

func TestExcludedTripLeavesWorkerQueue(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	u := uniq()
	cat := "exw-" + u
	pending, err := insertTrip(ctx, api, bootDraft(d, cat, "Ana "+u, 0, 8))
	if err != nil {
		t.Fatal(err)
	}
	// uma pronta, provisória e com recálculo, para os demais caminhos do worker
	ready, err := insertTrip(ctx, api, bootDraft(d, cat, "Beto "+u, 0, 9))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `UPDATE gold.trip SET status = 'ready', provisional = true, recalc_pending = true,
		first_seq = 1, last_seq = 10 WHERE trip_id = $1`, ready.ID); err != nil {
		t.Fatal(err)
	}
	worker := func(fn func(r port.Repositories) error) {
		t.Helper()
		if err := apiTx(ctx, owner, fn); err != nil {
			t.Fatal(err)
		}
	}
	has := func(ts []trip.Trip, id uuid.UUID) bool {
		for _, x := range ts {
			if x.ID == id {
				return true
			}
		}
		return false
	}
	worker(func(r port.Repositories) error {
		w, err := r.Trips.ListWaiting(ctx)
		if err != nil || !has(w, pending.ID) {
			t.Errorf("antes de excluir a pendente está na fila: %v", err)
		}
		return nil
	})
	var before trip.Queue
	worker(func(r port.Repositories) (err error) {
		before, err = r.Trips.CountQueue(ctx)
		return
	})

	if err := exclude(ctx, api, pending.ID); err != nil {
		t.Fatal(err)
	}
	if err := exclude(ctx, api, ready.ID); err != nil {
		t.Fatal(err)
	}
	worker(func(r port.Repositories) error {
		if w, err := r.Trips.ListWaiting(ctx); err != nil || has(w, pending.ID) {
			t.Errorf("ListWaiting: %v", err)
		}
		if _, found, err := r.Trips.Take(ctx, pending.ID); err != nil || found {
			t.Errorf("Take: %v %v", found, err)
		}
		if got, found, err := r.Trips.TakeRecalc(ctx); err != nil || (found && got.ID == ready.ID) {
			t.Errorf("TakeRecalc: %v %v", found, err)
		}
		p, err := r.Trips.ListProvisional(ctx)
		if err != nil {
			t.Errorf("ListProvisional: %v", err)
		}
		for _, x := range p {
			if x.ID == ready.ID {
				t.Error("ListProvisional traz a excluída")
			}
		}
		q, err := r.Trips.CountQueue(ctx)
		if err != nil || q.Waiting != before.Waiting-1 || q.Recalc != before.Recalc-1 {
			t.Errorf("CountQueue: %+v -> %+v (%v)", before, q, err)
		}
		return nil
	})
	// o reprocess não marca excluídas
	if _, err := owner.Exec(ctx, "UPDATE gold.trip SET recalc_pending = false WHERE trip_id = $1", ready.ID); err != nil {
		t.Fatal(err)
	}
	worker(func(r port.Repositories) error { return r.Trips.MarkRecalcByRange(ctx, d, 0, 100) })
	var marked bool
	if err := owner.QueryRow(ctx, "SELECT recalc_pending FROM gold.trip WHERE trip_id = $1", ready.ID).Scan(&marked); err != nil || marked {
		t.Errorf("MarkRecalcByRange marcou a excluída: %v %v", marked, err)
	}
}

func TestApiRoleCanOnlyExclude(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	tr, err := insertTrip(ctx, api, bootDraft(d, "exp-"+uniq(), "Ana", 0, 8))
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"UPDATE gold.trip SET status = 'ready' WHERE trip_id = $1",
		"UPDATE gold.trip SET driver_name = 'x' WHERE trip_id = $1",
		"UPDATE gold.trip SET boot_id = 9 WHERE trip_id = $1",
		"UPDATE gold.trip SET excluded_at = now(), status = 'ready' WHERE trip_id = $1",
	} {
		if _, err := api.Exec(ctx, sql, tr.ID); err == nil {
			t.Errorf("tccelta_api não deveria executar: %s", sql)
		}
	}
	if err := exclude(ctx, api, tr.ID); err != nil {
		t.Fatalf("a exclusão deve funcionar: %v", err)
	}
	var status string
	if err := owner.QueryRow(ctx, "SELECT status FROM gold.trip WHERE trip_id = $1", tr.ID).Scan(&status); err != nil || status != "pending" {
		t.Errorf("a exclusão não muda o status: %q %v", status, err)
	}
}

func TestExcludeWaitsForCalculationLock(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	tr, err := insertTrip(ctx, api, bootDraft(d, "exl-"+uniq(), "Ana", 0, 8))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx) // o worker calculando: segura FOR UPDATE na viagem
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT 1 FROM gold.trip WHERE trip_id = $1 FOR UPDATE", tr.ID); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = exclude(ctx, api, tr.ID)
	if !errors.Is(err, port.ErrDatabaseBusy) {
		t.Fatalf("esperava ErrDatabaseBusy: %v", err)
	}
	if el := time.Since(start); el < 2500*time.Millisecond {
		t.Errorf("deveria esperar o lock_timeout de 3 s, esperou %s", el)
	}
	// o cálculo termina: a exclusão passa
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := exclude(ctx, api, tr.ID); err != nil {
		t.Fatal(err)
	}
}

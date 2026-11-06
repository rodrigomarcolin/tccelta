//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/database"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
)

// Estes testes cobrem a migration 00011 (viagem repetida e listagem) e usam as mesmas variáveis TEST_BANCO_* dos demais.

// uniq devolve um sufixo único por chamada, para os nomes e categorias de um teste não colidirem com o banco compartilhado.
func uniq() string { return time.Now().Format("150405.000000") }

// bootDraft é draft com a geração resolvida e o boot escolhidos.
func bootDraft(dongle int32, category, name string, gen, boot int32) trip.Draft {
	d := draft(dongle, category, name)
	d.BootGeneration, d.BootID = &gen, &boot
	return d
}

func intervalDraft(dongle int32, category, name string, start, end time.Time) trip.Draft {
	return trip.Draft{Category: category, DriverName: name, DriverNameNormalized: trip.NormalizeName(name), Fuel: "diesel", DongleID: dongle,
		EntryType: trip.EntryTypeInterval, IntervalStart: &start, IntervalEnd: &end, TargetSeq: 5}
}

func insertTrip(ctx context.Context, api *pgxpool.Pool, d trip.Draft) (tr trip.Trip, err error) {
	err = apiTx(ctx, api, func(r port.Repositories) (err error) {
		tr, err = r.Trips.Insert(ctx, d)
		return
	})
	return
}

func findSame(ctx context.Context, api *pgxpool.Pool, d trip.Draft) (tr trip.Trip, found bool, err error) {
	err = apiTx(ctx, api, func(r port.Repositories) (err error) {
		tr, found, err = r.Trips.FindSame(ctx, d)
		return
	})
	return
}

func TestSameBootIsUnique(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	cat := "same-" + uniq()
	first, err := insertTrip(ctx, api, bootDraft(d, cat, "Ana", 0, 8))
	if err != nil {
		t.Fatal(err)
	}
	// outro motorista, e até outra categoria, no mesmo boot: o índice recusa
	for _, x := range []trip.Draft{bootDraft(d, cat, "Beto", 0, 8), bootDraft(d, cat+"-b", "Ana", 0, 8)} {
		if _, err := insertTrip(ctx, api, x); !errors.Is(err, trip.ErrTripRepeated) || errors.Is(err, trip.ErrDriverRepeated) {
			t.Fatalf("%v", err)
		}
	}
	// outra geração, outro boot_id e outro dongle são outras viagens
	for _, x := range []trip.Draft{bootDraft(d, cat, "Caio", 1, 8), bootDraft(d, cat, "Duda", 0, 9), bootDraft(newDongle(t, owner), cat, "Edu", 0, 8)} {
		if _, err := insertTrip(ctx, api, x); err != nil {
			t.Fatal(err)
		}
	}
	got, found, err := findSame(ctx, api, bootDraft(d, cat, "Qualquer", 0, 8))
	if err != nil || !found || got.ID != first.ID || got.DriverName != "Ana" {
		t.Fatalf("%v %v %+v", err, found, got)
	}
	if _, found, _ := findSame(ctx, api, bootDraft(d, cat, "Qualquer", 5, 8)); found {
		t.Fatal("outra geração não é a mesma viagem")
	}
	// failed libera
	if _, err := owner.Exec(ctx, "UPDATE gold.trip SET status = 'failed' WHERE trip_id = $1", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := findSame(ctx, api, bootDraft(d, cat, "Qualquer", 0, 8)); found {
		t.Fatal("failed não bloqueia")
	}
	second, err := insertTrip(ctx, api, bootDraft(d, cat, "Beto", 0, 8))
	if err != nil {
		t.Fatal(err)
	}
	// excluída libera
	if _, err := owner.Exec(ctx, "UPDATE gold.trip SET excluded_at = now() WHERE trip_id = $1", second.ID); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := findSame(ctx, api, bootDraft(d, cat, "Qualquer", 0, 8)); found {
		t.Fatal("excluída não bloqueia")
	}
	if _, err := insertTrip(ctx, api, bootDraft(d, cat, "Fabi", 0, 8)); err != nil {
		t.Fatal(err)
	}
}

func TestSameIntervalIsUnique(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	cat := "interval-" + uniq()
	t0 := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	first, err := insertTrip(ctx, api, intervalDraft(d, cat, "Ana", t0, t0.Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insertTrip(ctx, api, intervalDraft(d, cat, "Beto", t0, t0.Add(time.Hour))); !errors.Is(err, trip.ErrTripRepeated) {
		t.Fatalf("%v", err)
	}
	got, found, err := findSame(ctx, api, intervalDraft(d, cat, "Qualquer", t0.In(time.FixedZone("x", -3*3600)), t0.Add(time.Hour)))
	if err != nil || !found || got.ID != first.ID {
		t.Fatalf("o mesmo instante em outro fuso: %v %v %+v", err, found, got)
	}
	// só t0 e t1 exatamente iguais colidem
	for i, x := range []trip.Draft{intervalDraft(d, cat, "Caio", t0, t0.Add(2*time.Hour)), intervalDraft(d, cat, "Duda", t0.Add(time.Minute), t0.Add(time.Hour))} {
		if _, err := insertTrip(ctx, api, x); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
	}
	// um boot não colide com um intervalo
	if _, err := insertTrip(ctx, api, bootDraft(d, cat, "Edu", 0, 8)); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, "UPDATE gold.trip SET status = 'failed' WHERE trip_id = $1", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := insertTrip(ctx, api, intervalDraft(d, cat, "Fabi", t0, t0.Add(time.Hour))); err != nil {
		t.Fatalf("failed libera: %v", err)
	}
}

func TestConcurrentSameTripOneWins(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	cat := "race-" + uniq()
	for _, mk := range []func(name string) trip.Draft{
		func(name string) trip.Draft { return bootDraft(d, cat, name, 0, 8) },
		func(name string) trip.Draft {
			t0 := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
			return intervalDraft(d, cat+"-i", name, t0, t0.Add(time.Hour))
		},
	} {
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, name := range []string{"Ana " + uniq(), "Beto " + uniq()} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = insertTrip(ctx, api, mk(name))
			}()
		}
		wg.Wait()
		ok, repeated := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, trip.ErrTripRepeated):
				repeated++
			default:
				t.Fatal(err)
			}
		}
		if ok != 1 || repeated != 1 {
			t.Fatalf("ok=%d repetidos=%d, esperava 1 e 1", ok, repeated)
		}
		// a transação que perdeu abortou: o vencedor é lido em outra
		if w, found, err := findSame(ctx, api, mk("x")); err != nil || !found || w.ID == uuid.Nil {
			t.Fatalf("%v %v", err, found)
		}
	}
}

// migrationSection devolve o trecho Up ou Down de uma migration do goose.
func migrationSection(t *testing.T, file, section string) string {
	t.Helper()
	b, err := database.Migrations.ReadFile("migrations/" + file)
	if err != nil {
		t.Fatal(err)
	}
	up, down, ok := strings.Cut(string(b), "-- +goose Down")
	if !ok {
		t.Fatal("migration sem Down")
	}
	if section == "up" {
		return up
	}
	return down
}

func indexExists(ctx context.Context, tx pgx.Tx, name string) bool {
	var n int
	_ = tx.QueryRow(ctx, "SELECT count(*) FROM pg_indexes WHERE schemaname = 'gold' AND indexname = $1", name).Scan(&n)
	return n == 1
}

func ptr32(v int32) *int32 { return &v }

// A migration 00011 roda sobre um banco que já tem duplicatas: o teste desfaz os índices, cria as duplicatas, aplica o Up e
// confere quais viagens ficaram. Tudo numa transação que termina em rollback.
func TestMigration00011ExcludesDuplicates(t *testing.T) {
	ctx := context.Background()
	owner := connect(t, "TEST_BANCO_DONO_URL")
	d := newDongle(t, owner)
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	must := func(_ any, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, ix := range []string{"trip_boot_unique", "trip_interval_unique", "trip_list"} {
		must(tx.Exec(ctx, "DROP INDEX IF EXISTS gold."+ix))
	}
	for _, b := range []int32{5, 6, 7} {
		must(tx.Exec(ctx, "INSERT INTO boot (dongle_id, boot_generation, boot_id) VALUES ($1, 0, $2), ($1, 1, $2)", d, b))
	}
	base := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	n := 0
	add := func(entry string, gen, boot *int32, start, end *time.Time, status string, age int, excluded bool) uuid.UUID {
		t.Helper()
		n++
		var id uuid.UUID
		var ex *time.Time
		if excluded {
			ex = &base
		}
		name := "m" + uniq() + string(rune('a'+n))
		if err := tx.QueryRow(ctx, `INSERT INTO gold.trip (trip_category, driver_name, driver_name_normalized, fuel, dongle_id, entry_type,
			boot_generation, boot_id, interval_start, interval_end, status, target_seq, created_at, excluded_at)
			VALUES ($1, $2, $2, 'diesel', $3, $4, $5, $6, $7, $8, $9, 0, $10, $11) RETURNING trip_id`,
			"mig-"+name, name, d, entry, gen, boot, start, end, status, base.Add(time.Duration(age)*time.Minute), ex).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	g0, b5, b6, b7, b99 := int32(0), int32(5), int32(6), int32(7), int32(99)
	// boot 5: uma sem geração (vira a mais recente do boot, a 1), duas com a geração 1 (pendente e pronta) e uma com a 0
	nullGen := add("boot", nil, &b5, nil, nil, "pending", 0, false)
	gen1a := add("boot", ptr32(1), &b5, nil, nil, "pending", 1, false)
	gen1ready := add("boot", ptr32(1), &b5, nil, nil, "ready", 5, false)
	gen0 := add("boot", &g0, &b5, nil, nil, "pending", 2, false) // geração 0 é outra chave
	// boot 6: duas pendentes, vale a mais antiga
	old6 := add("boot", &g0, &b6, nil, nil, "pending", 1, false)
	new6 := add("boot", &g0, &b6, nil, nil, "waiting_data", 2, false)
	// boot 7: uma falhou e uma já estava excluída: nenhuma conta
	failed7 := add("boot", &g0, &b7, nil, nil, "failed", 0, false)
	excluded7 := add("boot", &g0, &b7, nil, nil, "pending", 1, true)
	live7 := add("boot", &g0, &b7, nil, nil, "pending", 2, false)
	// boot que ainda não existe: continua sem geração
	unknown := add("boot", nil, &b99, nil, nil, "pending", 0, false)
	// intervalos
	t0, t1 := base, base.Add(time.Hour)
	t1b := base.Add(2 * time.Hour)
	iOld := add("interval", nil, nil, &t0, &t1, "pending", 0, false)
	iNew := add("interval", nil, nil, &t0, &t1, "pending", 1, false)
	iReady := add("interval", nil, nil, &t0, &t1b, "ready", 9, false)
	iReadyDup := add("interval", nil, nil, &t0, &t1b, "pending", 0, false)

	must(tx.Exec(ctx, migrationSection(t, "00011_trip_unique_and_list.sql", "up"), pgx.QueryExecModeSimpleProtocol))

	excludedOf := func(id uuid.UUID) bool {
		var ex *time.Time
		if err := tx.QueryRow(ctx, "SELECT excluded_at FROM gold.trip WHERE trip_id = $1", id).Scan(&ex); err != nil {
			t.Fatal(err)
		}
		return ex != nil
	}
	for name, c := range map[string]struct {
		id   uuid.UUID
		want bool
	}{"boot sem geração (vira 1, perde para a pronta)": {nullGen, true}, "geração 1 pendente": {gen1a, true}, "geração 1 pronta": {gen1ready, false},
		"geração 0": {gen0, false}, "boot 6 mais antiga": {old6, false}, "boot 6 mais nova": {new6, true}, "failed": {failed7, false},
		"já excluída": {excluded7, true}, "viva ao lado da failed": {live7, false}, "boot desconhecido": {unknown, false},
		"intervalo mais antigo": {iOld, false}, "intervalo repetido": {iNew, true}, "intervalo pronto": {iReady, false}, "intervalo pronto repetido": {iReadyDup, true},
	} {
		if got := excludedOf(c.id); got != c.want {
			t.Errorf("%s: excluída=%v, esperava %v", name, got, c.want)
		}
	}
	var gen *int32
	if err := tx.QueryRow(ctx, "SELECT boot_generation FROM gold.trip WHERE trip_id = $1", nullGen).Scan(&gen); err != nil || gen == nil || *gen != 1 {
		t.Errorf("geração preenchida: %v %v", gen, err)
	}
	gen = nil
	if err := tx.QueryRow(ctx, "SELECT boot_generation FROM gold.trip WHERE trip_id = $1", unknown).Scan(&gen); err != nil || gen != nil {
		t.Errorf("boot desconhecido segue sem geração: %v %v", gen, err)
	}
	for _, ix := range []string{"trip_boot_unique", "trip_interval_unique", "trip_list"} {
		if !indexExists(ctx, tx, ix) {
			t.Errorf("falta o índice %s", ix)
		}
	}
	// o índice vale de verdade: um savepoint absorve a violação
	must(tx.Exec(ctx, "SAVEPOINT s"))
	_, err = tx.Exec(ctx, `INSERT INTO gold.trip (trip_category, driver_name, driver_name_normalized, fuel, dongle_id, entry_type, boot_generation, boot_id,
		status, target_seq) VALUES ('mig-x', 'x', 'x', 'diesel', $1, 'boot', 0, 6, 'pending', 0)`, d)
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.ConstraintName != "trip_boot_unique" {
		t.Errorf("esperava violação de trip_boot_unique: %v", err)
	}
	must(tx.Exec(ctx, "ROLLBACK TO SAVEPOINT s"))
	// o Down remove os índices e deixa as exclusões
	must(tx.Exec(ctx, migrationSection(t, "00011_trip_unique_and_list.sql", "down"), pgx.QueryExecModeSimpleProtocol))
	for _, ix := range []string{"trip_boot_unique", "trip_interval_unique", "trip_list"} {
		if indexExists(ctx, tx, ix) {
			t.Errorf("o Down deixou o índice %s", ix)
		}
	}
	if !excludedOf(new6) {
		t.Error("o Down não restaura as exclusões")
	}
}

// seedListed grava uma viagem boot com created_at fixo (o banco guarda microssegundos).
func seedListed(t *testing.T, owner *pgxpool.Pool, d int32, name string, boot int32, at time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := owner.QueryRow(context.Background(), `INSERT INTO gold.trip (trip_category, driver_name, driver_name_normalized, fuel, dongle_id,
		entry_type, boot_generation, boot_id, status, target_seq, created_at)
		VALUES ('lista-' || $1::text, $1, lower($1), 'diesel', $2, 'boot', 0, $3, 'pending', 0, $4) RETURNING trip_id`, name, d, boot, at).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func listAll(t *testing.T, r port.Trips, q trip.ListQuery) (ids []uuid.UUID, pages int) {
	t.Helper()
	for {
		p, err := r.List(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, it := range p.Items {
			ids = append(ids, it.ID)
		}
		if p.Next == nil {
			return
		}
		q.Cursor = p.Next
	}
}

func TestListCursorWithTiesAndNewTripBetweenBatches(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	tag := "lc" + time.Now().Format("150405000000")
	r := TripsWithPool(api)
	// 7 viagens, com 4 delas no mesmo created_at (empate): a ordem é (created_at, trip_id) decrescente
	at := time.Date(2026, 1, 1, 12, 0, 0, 123456000, time.UTC)
	want := map[uuid.UUID]bool{}
	for i := 0; i < 7; i++ {
		when := at
		if i >= 4 {
			when = at.Add(-time.Duration(i) * time.Second)
		}
		want[seedListed(t, owner, d, tag+"-"+string(rune('a'+i)), int32(10+i), when)] = true
	}
	q := trip.ListQuery{Limit: 3, Search: tag}
	p, err := r.List(ctx, q)
	if err != nil || p.Total != 7 || len(p.Items) != 3 || p.Next == nil {
		t.Fatalf("%v %+v", err, p)
	}
	if p.Next.CreatedAt.Nanosecond() != 123456000 {
		t.Fatalf("o cursor perdeu os microssegundos: %v", p.Next.CreatedAt)
	}
	first := []uuid.UUID{p.Items[0].ID, p.Items[1].ID, p.Items[2].ID}
	// uma viagem nova entra no topo entre os lotes: não repete nem pula nada
	newest := seedListed(t, owner, d, tag+"-new", 40, time.Now().UTC())
	q.Cursor = p.Next
	rest, _ := listAll(t, r, q)
	seen := map[uuid.UUID]bool{}
	for _, id := range append(first, rest...) {
		if seen[id] || id == newest {
			t.Fatalf("repetido ou novo demais: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != 7 {
		t.Fatalf("%d viagens vistas, esperava 7", len(seen))
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("faltou %s", id)
		}
	}
	// de novo do começo: 8 viagens, a nova primeiro
	q.Cursor = nil
	all, pages := listAll(t, r, q)
	if len(all) != 8 || pages != 3 || all[0] != newest {
		t.Fatalf("%d viagens em %d páginas, primeira %v", len(all), pages, all[0])
	}
	// nunca nula
	if p, err := r.List(ctx, trip.ListQuery{Limit: 5, Search: tag + "-inexistente"}); err != nil || p.Items == nil || p.Total != 0 || p.Next != nil {
		t.Fatalf("%v %+v", err, p)
	}
	// excluída some da lista e do total
	if _, err := owner.Exec(ctx, "UPDATE gold.trip SET excluded_at = now() WHERE trip_id = $1", newest); err != nil {
		t.Fatal(err)
	}
	if p, _ := r.List(ctx, trip.ListQuery{Limit: 50, Search: tag}); p.Total != 7 || len(p.Items) != 7 {
		t.Fatalf("%+v", p.Total)
	}
}

func TestListSearchEscapesWildcards(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	tag := "es" + time.Now().Format("150405000000")
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedListed(t, owner, d, tag+"100%x", 1, at)
	seedListed(t, owner, d, tag+"a_b", 2, at.Add(time.Second))
	seedListed(t, owner, d, tag+`c\d`, 3, at.Add(2*time.Second))
	seedListed(t, owner, d, tag+"azb", 4, at.Add(3*time.Second))
	r := TripsWithPool(api)
	for search, want := range map[string]int64{tag: 4, tag + "100%": 1, tag + "a_b": 1, tag + `c\d`: 1, tag + "a": 2, "%" + tag: 0, tag + "%": 0, tag + "_": 0} {
		p, err := r.List(ctx, trip.ListQuery{Limit: 50, Search: trip.NormalizeName(search)})
		if err != nil || p.Total != want || int64(len(p.Items)) != want {
			t.Errorf("%q: total=%d itens=%d (esperava %d) err=%v", search, p.Total, len(p.Items), want, err)
		}
	}
}

func TestListSummaryAndRoute(t *testing.T) {
	ctx := context.Background()
	owner, api := connect(t, "TEST_BANCO_DONO_URL"), connect(t, "TEST_BANCO_API_URL")
	d := newDongle(t, owner)
	tag := "sm" + time.Now().Format("150405000000")
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	pending := seedListed(t, owner, d, tag+"-p", 1, at)
	noSummary := seedListed(t, owner, d, tag+"-n", 2, at.Add(time.Second))
	ready := seedListed(t, owner, d, tag+"-r", 3, at.Add(2*time.Second))
	noGPS := seedListed(t, owner, d, tag+"-g", 4, at.Add(3*time.Second))
	q := dbgen.New(owner)
	start, end := at.Add(-time.Hour), at.Add(-30*time.Minute)
	save := func(id uuid.UUID, dist float64, route string) {
		t.Helper()
		if err := q.InsertTripSummary(ctx, dbgen.InsertTripSummaryParams{TripID: id, Warnings: []byte(`[]`), Parameters: []byte(`{}`), PidPeriodsMs: []byte(`{}`),
			DistanceKm: &dist, DurationS: ptr(1800.0), StartedAt: &start, EndedAt: &end}); err != nil {
			t.Fatal(err)
		}
		if err := q.InsertTripSeries(ctx, dbgen.InsertTripSeriesParams{TripID: id, StepMs: 100, ExposureStepMs: 100, Series: []byte(`{}`), SlowSeries: []byte(`{}`),
			Route: []byte(route), RpmDistribution: []byte(`[]`), PedalDistribution: []byte(`[]`), RpmPedalMatrix: []byte(`{}`)}); err != nil {
			t.Fatal(err)
		}
		if err := q.SaveTripResult(ctx, dbgen.SaveTripResultParams{TripID: id}); err != nil {
			t.Fatal(err)
		}
	}
	save(ready, 12.3, `{"t_ms":[0,1000,2000,3000],"lat":[-23.5,-23.51,-23.6,-23.61],"lon":[-46.6,-46.61,-46.7,-46.71],`+
		`"segmentos":{"velocidade_kmh":[10,null,10],"faixa":["v1","falha_gps","v1"]}}`)
	save(noGPS, 1, `{"t_ms":[],"lat":[],"lon":[],"segmentos":{"velocidade_kmh":[],"faixa":[]}}`)
	p, err := TripsWithPool(api).List(ctx, trip.ListQuery{Limit: 10, Search: tag})
	if err != nil || len(p.Items) != 4 {
		t.Fatalf("%v %+v", err, p)
	}
	by := map[uuid.UUID]trip.ListItem{}
	for _, it := range p.Items {
		by[it.ID] = it
	}
	for _, id := range []uuid.UUID{pending, noSummary} {
		it := by[id]
		if it.Route != nil || it.DistanceKm != nil || it.DurationS != nil || it.StartedAt != nil || it.EndedAt != nil {
			t.Errorf("sem resumo: %+v", it)
		}
	}
	it := by[ready]
	if it.Status != trip.StatusReady || *it.DistanceKm != 12.3 || *it.DurationS != 1800 || !it.StartedAt.Equal(start) || len(it.Route) != 2 {
		t.Errorf("pronta com GPS: %+v", it)
	}
	if it := by[noGPS]; it.Route != nil || *it.DistanceKm != 1 {
		t.Errorf("pronta sem GPS: %+v", it)
	}
}

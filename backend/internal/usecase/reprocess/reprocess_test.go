package reprocess

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"tccelta/backend/internal/adapter/memory"
	"tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/port/porttest"
)

var t0 = time.Unix(1_800_000_000, 0)

const dongleID = 7

func f(v float64) *float64 { return &v }
func i64(v int64) *int64   { return &v }

type metrics struct {
	porttest.Nop
	rows int
}

func (m *metrics) Reprocessed(n int) { m.rows += n }

// dataBlock monta um bloco de dados (PID 12 + GPS) como o servidor o guarda em raw_block.
func dataBlock(seq int64, boot int32, pidID uint8) ingestion.StoredBlock {
	m := protocol.NewBlockBuilder(uint32(seq), uint16(boot), 1000)
	m.PID(pidID, 0, 0x17, 0x70)
	m.GPS(10, -235505200, -466333100, 760000)
	return ingestion.StoredBlock{Seq: seq, BootID: boot, T0Ms: 1000, Payload: m.Data(), ReceivedAt: t0.Add(time.Duration(seq) * time.Second)}
}

type env struct {
	db  *memory.Database
	uc  *Reprocessor
	met *metrics
}

func newEnv(t *testing.T, blocks ...ingestion.StoredBlock) *env {
	t.Helper()
	db := memory.New()
	db.DataBlocks[dongleID] = blocks
	cat := pid.FromMap([]pid.Def{{PID: 12, NBytes: 2, Factor: 0.25, Min: f(0), Max: f(16383.75)}})
	m := &metrics{}
	return &env{db: db, met: m,
		uc: New(db.Repositories(), db, cat, m, slog.New(slog.NewTextHandler(io.Discard, nil)))}
}

func (e *env) seed(seq int64) {
	e.db.Readings = append(e.db.Readings, pid.Reading{BlockSeq: seq, PID: 12, TMs: 777})
}

func TestDryRunWritesNothing(t *testing.T) {
	e := newEnv(t, dataBlock(1, 1, 12), dataBlock(2, 1, 12), dataBlock(3, 1, 12))
	e.seed(1)
	e.seed(2)
	r, err := e.uc.Run(context.Background(), Options{DongleID: dongleID})
	if err != nil {
		t.Fatal(err)
	}
	if r.Blocks != 3 || r.RemovedRows != 2 || r.InsertedRows != 3 || r.LastSeq != 3 {
		t.Fatalf("%+v", r)
	}
	if len(e.db.Readings) != 2 || len(e.db.Positions) != 0 || len(e.db.Alerts) != 0 || e.met.rows != 0 {
		t.Fatalf("o ensaio gravou: %d leituras, %d posições, %d alertas", len(e.db.Readings), len(e.db.Positions), len(e.db.Alerts))
	}
}

func TestApplyReplacesRowsAndMarksGold(t *testing.T) {
	e := newEnv(t, dataBlock(1, 1, 12), dataBlock(2, 1, 12), dataBlock(3, 1, 12))
	e.seed(1)
	e.seed(2)
	e.seed(9) // fora da faixa dos blocos: fica
	boot := &memory.Boot{LastSeq: i64(3)}
	boot.BootKey = timeline.BootKey{DongleID: dongleID, BootID: 1}
	boot.FirstSeq = i64(1)
	other := &memory.Boot{LastSeq: i64(100)}
	other.BootKey = timeline.BootKey{DongleID: dongleID, BootID: 2}
	other.FirstSeq = i64(50)
	e.db.BootRows = append(e.db.BootRows, boot, other)

	r, err := e.uc.Run(context.Background(), Options{DongleID: dongleID, Apply: true, Operator: "teste", Batch: 2})
	if err != nil {
		t.Fatal(err)
	}
	// 2 janelas (blocos 1-2 e 3): a 1ª remove 2 e insere 2, a 2ª não remove e insere 1
	if r.Blocks != 3 || r.RemovedRows != 2 || r.InsertedRows != 3 || r.LastSeq != 3 {
		t.Fatalf("%+v", r)
	}
	if len(e.db.Readings) != 4 || len(e.db.Positions) != 3 {
		t.Fatalf("%d leituras, %d posições", len(e.db.Readings), len(e.db.Positions))
	}
	if e.met.rows != 3 {
		t.Fatalf("métrica %d", e.met.rows)
	}
	if !boot.GoldPending || other.GoldPending {
		t.Fatalf("gold: %v %v", boot.GoldPending, other.GoldPending)
	}
	if len(e.db.Alerts) != 1 || e.db.Alerts[0].Type != "reprocessing" || !strings.Contains(string(e.db.Alerts[0].Detail), `"operator":"teste"`) {
		t.Fatalf("%+v", e.db.Alerts)
	}
}

// PID fora do catálogo aborta a janela sem tocar nas linhas existentes.
func TestUnknownPidAbortsWindow(t *testing.T) {
	e := newEnv(t, dataBlock(1, 1, 12), dataBlock(2, 1, 99))
	e.seed(1)
	_, err := e.uc.Run(context.Background(), Options{DongleID: dongleID, Apply: true})
	if err == nil || !strings.Contains(err.Error(), "seq 2") {
		t.Fatalf("%v", err)
	}
	if len(e.db.Readings) != 1 || e.db.Readings[0].TMs != 777 || len(e.db.Alerts) != 0 {
		t.Fatalf("alterou algo: %+v %+v", e.db.Readings, e.db.Alerts)
	}
}

func TestRange(t *testing.T) {
	e := newEnv(t, dataBlock(1, 1, 12), dataBlock(2, 1, 12), dataBlock(3, 1, 12))
	r, err := e.uc.Run(context.Background(), Options{DongleID: dongleID, FromSeq: 2, ToSeq: 2})
	if err != nil || r.Blocks != 1 || r.LastSeq != 2 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestNoBlocks(t *testing.T) {
	e := newEnv(t)
	r, err := e.uc.Run(context.Background(), Options{DongleID: dongleID, Apply: true})
	if err != nil || r.Blocks != 0 || len(e.db.Alerts) != 1 {
		t.Fatalf("%+v %v alertas=%d", r, err, len(e.db.Alerts))
	}
}

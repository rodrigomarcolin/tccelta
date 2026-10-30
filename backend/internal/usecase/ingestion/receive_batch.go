// Package ingestion tem os casos de uso do lote de blocos (spec 05).
package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"tccelta/backend/internal/domain/dongle"
	ding "tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/port"
)

// Params são os ajustes da ingestão.
type Params struct {
	MaxBatch         int
	VerifyBatch      int
	VerifyParallel   int
	AllowNoneProfile bool
}

// ReceiveBatchInput é o lote recebido: a sessão autenticada e o corpo bruto (TLVs de bloco).
type ReceiveBatchInput struct {
	Session port.Session
	Body    []byte
}

// BatchResult é a saída de ReceiveBatch.
type BatchResult struct {
	AckTLV      []byte
	AckSeq      uint32
	Interrupted bool
	Reason      ding.Reason
	Action      ding.Action
	NoProgress  bool
	Accepted    int
	Duplicates  int
}

// ReceiveBatch implementa o algoritmo da spec 05 (POST /v1/dongles/blocos).
type ReceiveBatch struct {
	repos port.Repositories
	tx    port.Transactor
	kms   port.Authenticator
	pids  pid.Query
	met   port.MetricsIngestion
	now   port.Clock
	log   *slog.Logger
	p     Params
}

// NewReceiveBatch liga o caso de uso às suas portas.
func NewReceiveBatch(repos port.Repositories, tx port.Transactor, kms port.Authenticator, pids pid.Query,
	met port.MetricsIngestion, now port.Clock, log *slog.Logger, p Params) *ReceiveBatch {
	return &ReceiveBatch{repos: repos, tx: tx, kms: kms, pids: pids, met: met, now: now, log: log, p: p}
}

func ackDecoded(tlv []byte) (protocol.Ack, bool) {
	t, n, err := protocol.ReadTLV(tlv)
	if err != nil || n != len(tlv) || t.T != protocol.AckType {
		return protocol.Ack{}, false
	}
	a, _, err := protocol.DecodeAck(t.V)
	return a, err == nil
}

// Run processa o lote.
func (c *ReceiveBatch) Run(ctx context.Context, e ReceiveBatchInput) (BatchResult, error) {
	d, err := c.repos.Dongles.FindByDevID(ctx, e.Session.Dev)
	if err != nil {
		return BatchResult{}, err
	}
	if err := d.VerifyAccess(c.p.AllowNoneProfile); err != nil {
		return BatchResult{}, err
	}

	// 1. forma
	tlvs, err := protocol.ReadBatch(e.Body, c.p.MaxBatch)
	if err != nil {
		if errors.Is(err, protocol.ErrBatchTooLarge) {
			return BatchResult{}, ding.ErrBatchTooLarge
		}
		return BatchResult{}, fmt.Errorf("%w: %v", ding.ErrMalformedBatch, err)
	}
	blocks := make([]ding.BlockIn, len(tlvs))
	for i, t := range tlvs {
		if t.T != protocol.BlockType {
			return BatchResult{}, fmt.Errorf("%w: tipo 0x%02x", ding.ErrMalformedBatch, uint8(t.T))
		}
		b, err := protocol.DecodeBlock(t.V, d.Profile)
		if err != nil {
			return BatchResult{}, fmt.Errorf("%w: %v", ding.ErrMalformedBatch, err)
		}
		blocks[i].B = b
	}

	// 2. snapshot
	snap, err := c.repos.States.ReadSnapshot(ctx, d.ID)
	if err != nil {
		return BatchResult{}, err
	}
	var idx []int // blocos a verificar (seq > snapshot, sem repetir seq)
	seen := map[uint32]bool{}
	for i := range blocks {
		sq := blocks[i].Seq()
		if int64(sq) > snap.ContiguousSeq && !seen[sq] {
			seen[sq] = true
			idx = append(idx, i)
		}
	}
	// repetições do mesmo seq dentro do lote herdam o resultado do primeiro
	for i := range blocks {
		if int64(blocks[i].Seq()) > snap.ContiguousSeq {
			blocks[i].Verified = true
		}
	}

	// 3. KMS em paralelo (verify do lote + pré-assinatura do ACK provável)
	valid := map[uint32]bool{}
	var tagPre []byte
	var ackPre protocol.Ack
	presigned := false
	g, gctx := errgroup.WithContext(port.WithKMSPhase(ctx, port.PhasePre))
	if len(idx) > 0 {
		batch := c.p.VerifyBatch
		if batch <= 0 {
			batch = 32
		}
		par := c.p.VerifyParallel
		if par <= 0 {
			par = 1
		}
		var without = make(chan struct{}, par)
		var res = make([]bool, len(idx))
		for start := 0; start < len(idx); start += batch {
			end := min(start+batch, len(idx))
			start, end := start, end
			g.Go(func() error {
				without <- struct{}{}
				defer func() { <-without }()
				inputs := make([][]byte, 0, end-start)
				tags := make([][]byte, 0, end-start)
				for _, i := range idx[start:end] {
					inputs = append(inputs, protocol.TagInput(protocol.BlockType, d.DevID, blocks[i].B.Data))
					tags = append(tags, blocks[i].B.Tag)
				}
				out, err := c.kms.VerifyBlocks(gctx, d.RecKey, inputs, tags)
				if err != nil {
					return err
				}
				copy(res[start:end], out)
				return nil
			})
		}
		g.Go(func() error {
			// ACK provável: simula com todas as tags válidas sobre o snapshot
			sim := make([]ding.BlockIn, len(blocks))
			copy(sim, blocks)
			for i := range sim {
				sim[i].Valid = true
			}
			r := ding.Apply(sim, ding.StateIn{ContiguousSeq: uint32(snap.ContiguousSeq)}, c.pids)
			prov := protocol.Ack{AckSeq: r.NewAck, CConf: uint32(snap.CConf)}
			if a, ok := ackDecoded(snap.LastAck); ok && a == prov {
				return nil
			}
			macs, err := c.kms.HMACAuth(gctx, d.AuthKey, [][]byte{protocol.TagInput(protocol.AckType, d.DevID, prov.Payload())})
			if err != nil {
				return err
			}
			tagPre, ackPre, presigned = macs[0][:protocol.AuthTagSize], prov, true
			return nil
		})
		if err := g.Wait(); err != nil {
			return BatchResult{}, err
		}
		for k, i := range idx {
			valid[blocks[i].Seq()] = res[k]
		}
	}
	for i := range blocks {
		if blocks[i].Verified {
			blocks[i].Valid = valid[blocks[i].Seq()]
		}
	}

	// 4. transação
	var out BatchResult
	t0tx := time.Now()
	err = c.tx.InTransaction(ctx, func(ctx context.Context, rp port.Repositories) error {
		out = BatchResult{}
		if err := rp.Dongles.Lock(ctx, d.ID); err != nil {
			return err
		}
		est, err := rp.States.Read(ctx, d.ID)
		if err != nil {
			return err
		}
		now := c.now()
		r := ding.Apply(blocks, ding.StateIn{ContiguousSeq: uint32(est.ContiguousSeq), G: uint16(est.BootGeneration), U: uint16(est.LastBootID)}, c.pids)

		for _, rej := range r.Rejected {
			if err := c.reject(ctx, rp, d, e.Session, rej, now); err != nil {
				return err
			}
			if rej.Reason == ding.ReasonUnknownPID {
				det, _ := json.Marshal(map[string]any{"pid": rej.PID, "seq": rej.B.Hdr.Seq})
				if err := rp.Occurrences.InsertAlert(ctx, occurrence.Alert{DongleID: d.ID, Type: occurrence.AlertUnknownPID, Detail: det}); err != nil {
					return err
				}
			}
		}

		if len(r.Accepted) > 0 {
			if err := c.store(ctx, rp, d, e.Session, est, r, now); err != nil {
				return err
			}
		}

		ackCurrent, err := rp.States.ReadLastAck(ctx, d.ID)
		if err != nil {
			return err
		}
		ack := protocol.Ack{AckSeq: r.NewAck, CConf: uint32(est.CConf)}
		var ackTLV []byte
		if a, ok := ackDecoded(ackCurrent); ok && a == ack {
			ackTLV = ackCurrent
		} else if presigned && ackPre == ack {
			ackTLV = ack.Build(tagPre)
		} else {
			macs, err := c.kms.HMACAuth(port.WithKMSPhase(ctx, port.PhaseTx), d.AuthKey,
				[][]byte{protocol.TagInput(protocol.AckType, d.DevID, ack.Payload())})
			if err != nil {
				return err
			}
			ackTLV = ack.Build(macs[0][:protocol.AuthTagSize])
		}
		if int64(r.NewAck) != est.ContiguousSeq || int64(r.MaxSeen) > est.MaxSeenSeq || int32(r.G) != est.BootGeneration ||
			string(ackTLV) != string(ackCurrent) {
			if err := rp.States.UpdateIngestion(ctx, ding.StateUpdate{DongleID: d.ID, ContiguousSeq: int64(r.NewAck),
				MaxSeen: int64(r.MaxSeen), LastAck: ackTLV, Now: now, BootGeneration: int32(r.G), LastBootID: int32(r.U)}); err != nil {
				return err
			}
		}
		out = BatchResult{AckTLV: ackTLV, AckSeq: r.NewAck, Reason: r.Reason, Action: ding.ActionOf(r.Reason),
			Accepted: len(r.Accepted), Duplicates: r.Duplicates}
		last := blocks[len(blocks)-1].Seq()
		out.Interrupted = r.NewAck < last
		out.NoProgress = int64(r.NewAck) == est.ContiguousSeq && r.Reason == ding.ReasonInvalidTag
		c.count(r)
		return nil
	})
	c.met.IngestionTxDuration(time.Since(t0tx))
	if err != nil {
		return BatchResult{}, err
	}
	st := "completo"
	if out.NoProgress {
		st = "sem_progresso"
	} else if out.Interrupted {
		st = "interrompido"
	}
	c.met.Batch(st)
	if out.Interrupted {
		c.log.Warn("lote.interrompido", "dongle", d.DevID.Hex(), "motivo", string(out.Reason), "ack_seq", out.AckSeq)
	}
	return out, nil
}

func (c *ReceiveBatch) count(r ding.ApplyResult) {
	c.met.Blocks("accepted", len(r.Accepted))
	c.met.Blocks("duplicate", r.Duplicates)
	for _, rej := range r.Rejected {
		c.met.Blocks(string(rej.Reason), 1)
	}
	if r.Reason == ding.ReasonJump {
		c.met.Blocks("discarded_after_interruption", 1)
	}
}

func (c *ReceiveBatch) reject(ctx context.Context, rp port.Repositories, d dongle.Dongle, session port.Session,
	rej ding.Rejection, now time.Time) error {
	h := sha256.Sum256(append(append([]byte{}, rej.B.Data...), rej.B.Tag...))
	return rp.Blocks.RegisterRejected(ctx, ding.Rejected{DongleID: d.ID, Seq: int64(rej.B.Hdr.Seq), BootID: int32(rej.B.Hdr.BootID),
		T0Ms: int64(rej.B.Hdr.T0Ms), Payload: rej.B.Data, Signature: rej.B.Tag, Reason: rej.Reason, Detail: rej.Detail,
		ContentHash: h[:], TokenJTI: session.JTI, ReceivedAt: now})
}

type bootKey struct{ g, b int32 }

type aggBoot struct {
	seqStart, seqEnd, tStart, tEnd int64
	offset                         float64
}

// store persiste os blocos aceitos e tudo que deriva deles.
func (c *ReceiveBatch) store(ctx context.Context, rp port.Repositories, d dongle.Dongle, session port.Session,
	est dongle.State, r ding.ApplyResult, now time.Time) error {
	// referências de t_ms por boot (gerações G e G+1)
	var ids []int32
	seenID := map[int32]bool{}
	for _, a := range r.Accepted {
		if a.Marker == "" && !seenID[int32(a.B.Hdr.BootID)] {
			seenID[int32(a.B.Hdr.BootID)] = true
			ids = append(ids, int32(a.B.Hdr.BootID))
		}
	}
	ref := map[bootKey]int64{}
	if len(ids) > 0 {
		rows, err := rp.Boots.LastTMs(ctx, d.ID, ids, est.BootGeneration, est.BootGeneration+1)
		if err != nil {
			return err
		}
		for _, x := range rows {
			ref[bootKey{x.Generation, x.BootID}] = x.LastTMs
		}
	}
	raw := make([]ding.RawBlock, len(r.Accepted))
	var readings []pid.Reading
	var positions []pid.Position
	agg := map[bootKey]*aggBoot{}
	recMs := float64(now.UnixMicro()) / 1000
	for i, a := range r.Accepted {
		hdr := a.B.Hdr
		t0 := int64(hdr.T0Ms)
		if a.Marker == "" {
			k := bootKey{int32(a.Generation), int32(hdr.BootID)}
			rf, ok := ref[k]
			if !ok {
				rf = -1
			}
			t0 = timeline.Unwrap(hdr.T0Ms, rf)
			ls, ps, err := pid.Decode(a.Generation, hdr, a.Content, t0, c.pids)
			if err != nil {
				return err
			}
			readings = append(readings, ls...)
			positions = append(positions, ps...)
			tStart, tEnd := t0+int64(a.Content.DtMin), t0+int64(a.Content.DtMax)
			ref[k] = tEnd
			offset := recMs - float64(tEnd)
			if x, ok := agg[k]; ok {
				x.seqEnd = int64(hdr.Seq)
				x.tStart, x.tEnd = min(x.tStart, tStart), max(x.tEnd, tEnd)
				x.offset = min(x.offset, offset)
			} else {
				agg[k] = &aggBoot{seqStart: int64(hdr.Seq), seqEnd: int64(hdr.Seq), tStart: tStart, tEnd: tEnd, offset: offset}
			}
		}
		raw[i] = ding.RawBlock{Seq: int64(hdr.Seq), BootID: int32(hdr.BootID), Generation: int32(a.Generation), T0Ms: t0,
			Marker: a.Marker, Payload: a.B.Data, Signature: a.B.Tag}
	}
	stored, err := rp.Blocks.InsertRawBlocks(ctx, d.ID, int64(session.CS), now, raw)
	if err != nil {
		return err
	}
	if stored != len(raw) {
		det, _ := json.Marshal(map[string]any{"esperado": len(raw), "gravado": stored})
		_ = rp.Occurrences.InsertAlert(ctx, occurrence.Alert{DongleID: d.ID, Type: occurrence.AlertInvariantViolated, Detail: det})
		return ding.ErrInvariant
	}
	if len(readings) > 0 {
		if err := rp.Blocks.CopyReadings(ctx, d.ID, now, readings); err != nil {
			return err
		}
	}
	if len(positions) > 0 {
		if err := rp.Blocks.CopyPositions(ctx, d.ID, now, positions); err != nil {
			return err
		}
	}
	c.met.Rows("reading", len(readings))
	c.met.Rows("position", len(positions))
	for _, l := range r.Gaps {
		sb := int64(l.BlockSeq)
		if err := rp.Occurrences.InsertGap(ctx, occurrence.Gap{DongleID: d.ID, From: int64(l.From), To: int64(l.To),
			Cause: l.Cause, RegisteredAt: now, BlockSeq: &sb}); err != nil {
			return err
		}
		c.met.Gap(l.Cause)
	}
	if int32(r.G) > est.BootGeneration {
		det, _ := json.Marshal(map[string]any{"from": est.BootGeneration, "to": r.G, "last_boot_id": est.LastBootID})
		if err := rp.Occurrences.InsertAlert(ctx, occurrence.Alert{DongleID: d.ID, Type: occurrence.AlertBootGenerationAdvanced, Detail: det}); err != nil {
			return err
		}
	}
	if len(agg) > 0 {
		keys := make([]bootKey, 0, len(agg))
		for k := range agg {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].g != keys[j].g {
				return keys[i].g < keys[j].g
			}
			return keys[i].b < keys[j].b
		})
		aggs := make([]ding.BootAggregate, 0, len(keys))
		for _, k := range keys {
			x := agg[k]
			aggs = append(aggs, ding.BootAggregate{Generation: k.g, BootID: k.b, FromSeq: x.seqStart, ToSeq: x.seqEnd,
				TStart: x.tStart, TEnd: x.tEnd, MaxOffset: x.offset})
		}
		if err := rp.Boots.UpdateIngestion(ctx, d.ID, now, aggs); err != nil {
			return err
		}
		if err := rp.Boots.NotifyPending(ctx, fmt.Sprintf("%d", d.ID)); err != nil {
			return err
		}
	}
	return nil
}

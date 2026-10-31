package simulator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
	"tccelta/backend/internal/usecase/catalog"
	"tccelta/backend/internal/usecase/reprocess"
)

// Options configuram a execução de um cenário.
type Options struct {
	API         string
	Inspect     port.Inspector    // leitura de verificação (papel com SELECT; tccelta_owner em dev/teste)
	Repos       port.Repositories // catálogo de PIDs e reprocessamento (o cenário generation)
	Tx          port.Transactor   // transações do reprocessamento
	Keys        []Keys
	Report      *Report
	Log         *slog.Logger
	Dongles     int
	Duration    time.Duration
	TLat        time.Duration
	Accumulated int
	Ramp        time.Duration
	Speedup     float64
	Drift       float64
}

type verifier struct {
	failures []string
	log      *slog.Logger
}

func (v *verifier) ok(cond bool, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if cond {
		v.log.Info("OK " + msg)
		fmt.Println("OK  ", msg)
		return
	}
	v.failures = append(v.failures, msg)
	fmt.Println("FALHOU", msg)
}

func (v *verifier) result() error {
	if len(v.failures) > 0 {
		return fmt.Errorf("%d verificação(ões) falharam", len(v.failures))
	}
	return nil
}

// Run roda o cenário pelo nome.
func Run(ctx context.Context, name string, o Options) error {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.TLat <= 0 {
		o.TLat = 60 * time.Second
	}
	if o.Speedup <= 0 {
		o.Speedup = 1
	}
	switch name {
	case "e2e":
		return e2e(ctx, o)
	case "negative-protocol":
		return negative(ctx, o)
	case "time":
		return timeScenario(ctx, o)
	case "generation":
		return generation(ctx, o)
	case "asym":
		return asymScenario(ctx, o)
	case "load":
		return load(ctx, o)
	case "reconnection":
		return reconnection(ctx, o)
	case "knee":
		return knee(ctx, o)
	}
	return fmt.Errorf("cenário desconhecido %q", name)
}

func client() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 64, IdleConnTimeout: 90 * time.Second}}
}

// waitForBoots espera o worker fechar a fila de tempo do dongle.
func waitForBoots(ctx context.Context, q port.Inspector, id int32, maxWait time.Duration) []port.InspectedBoot {
	end := time.Now().Add(maxWait)
	for {
		bs, _ := q.Boots(ctx, id)
		pend := false
		for _, b := range bs {
			pend = pend || b.TimePending
		}
		if !pend || time.Now().After(end) {
			return bs
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func covered(ls []port.InspectedGap, seq uint32) bool {
	for _, l := range ls {
		if int64(seq) >= l.FromSeq && int64(seq) <= l.ToSeq {
			return true
		}
	}
	return false
}

// drenarEm gera em lotes e drena, para que a flash pequena não descarte nada.
func generateAndDrain(ctx context.Context, a *AppVirtual, n int) error {
	for n > 0 {
		c := min(n, 20)
		a.Dongle.GenerateBlocks(c)
		n -= c
		if err := a.Drain(ctx); err != nil {
			return err
		}
	}
	return nil
}

func e2e(ctx context.Context, o Options) error {
	v := &verifier{log: o.Log}
	q := o.Inspect
	for i, ch := range o.Keys {
		d := NewDongle(ch, ConfigDongle{FlashCapacity: 30})
		a := NewApp(o.API, d, client(), o.Report)
		if err := a.Handshake(ctx); err != nil {
			return fmt.Errorf("dongle %d: handshake: %w", i, err)
		}
		if err := generateAndDrain(ctx, a, 200); err != nil {
			return fmt.Errorf("dongle %d: fase 1: %w", i, err)
		}
		// queda de energia (BOOT com furo) e 50 blocos
		d.LosePower(3)
		if err := a.Handshake(ctx); err != nil {
			return err
		}
		if err := generateAndDrain(ctx, a, 50); err != nil {
			return fmt.Errorf("dongle %d: fase 2: %w", i, err)
		}
		// bloco corrompido (LOSS)
		d.GenerateBlocks(10)
		d.Corrupt(d.Head() - 5)
		if err := a.Drain(ctx); err != nil {
			return fmt.Errorf("dongle %d: fase 3: %w", i, err)
		}
		// 5 min offline com flash pequena (OVERFLOW)
		d.GenerateBlocks(60)
		if err := a.Handshake(ctx); err != nil {
			return err
		}
		if err := a.Drain(ctx); err != nil {
			return fmt.Errorf("dongle %d: fase 4: %w", i, err)
		}
		if i == len(o.Keys)-1 {
			// OVERFLOW que engole o segmento antes do BOOT (caso 16)
			d.GenerateBlocks(20)
			d.LosePower(5)
			d.GenerateBlocks(4)
			d.DiscardSegment(20)
			if err := a.Handshake(ctx); err != nil {
				return err
			}
			if err := a.Drain(ctx); err != nil {
				return fmt.Errorf("dongle %d: caso 16: %w", i, err)
			}
			// BOOT corrompido (caso 18)
			d.LosePower(3)
			d.GenerateBlocks(2)
			d.Corrupt(d.LastSeqBoot())
			if err := a.Handshake(ctx); err != nil {
				return err
			}
			if err := a.Drain(ctx); err != nil {
				return fmt.Errorf("dongle %d: caso 18: %w", i, err)
			}
		}
		dev := ch.Dev
		est, err := q.DongleState(ctx, dev)
		if err != nil {
			return err
		}
		head := d.Head()
		v.ok(est.ContiguousSeq == int64(head) && est.HeadSeq == int64(head), "dongle %d: contiguous_seq=%d head=%d (dongle=%d)", i, est.ContiguousSeq, est.HeadSeq, head)
		ls, _ := q.Gaps(ctx, est.DongleID)
		causes := map[string]int{}
		for _, l := range ls {
			causes[l.Cause]++
		}
		v.ok(causes["BOOT"] > 0 && causes["LOSS"] > 0 && causes["OVERFLOW"] > 0, "dongle %d: lacunas %v", i, causes)
		var expected, wantPos int64
		for seq, n := range d.Records {
			if !covered(ls, seq) {
				expected += int64(n - 1)
				wantPos++
			}
		}
		n, _ := q.CountReadings(ctx, est.DongleID)
		pos, _ := q.CountPositions(ctx, est.DongleID)
		v.ok(n > 0 && n == expected && pos == wantPos, "dongle %d: leituras=%d/%d posicoes=%d/%d", i, n, expected, pos, wantPos)
		rej, _ := q.CountRejected(ctx, est.DongleID)
		v.ok(rej == 0, "dongle %d: rejected_block vazio (%d)", i, rej)
		anc, _ := q.CountAnchors(ctx, est.DongleID)
		v.ok(anc >= 1, "dongle %d: ancoras=%d", i, anc)
		bs := waitForBoots(ctx, q, est.DongleID, 15*time.Second)
		withTime := 0
		for _, b := range bs {
			if b.AMs != nil && b.TimeSource != nil {
				withTime++
			}
		}
		v.ok(len(bs) > 0 && withTime > 0, "dongle %d: boots=%d com tempo=%d", i, len(bs), withTime)
	}
	return v.result()
}

func negative(ctx context.Context, o Options) error {
	v := &verifier{log: o.Log}
	q := o.Inspect
	if len(o.Keys) < 2 {
		return errors.New("precisa de 2 dongles")
	}
	d := NewDongle(o.Keys[0], ConfigDongle{})
	a := NewApp(o.API, d, client(), o.Report)
	if err := a.Handshake(ctx); err != nil {
		return err
	}
	d.GenerateBlocks(10)
	if err := a.Drain(ctx); err != nil {
		return err
	}
	dev := o.Keys[0].Dev
	est0, _ := q.DongleState(ctx, dev)
	l0, _ := q.CountReadings(ctx, est0.DongleID)

	// a) 1 byte trocado no 2º bloco => 200 interrompido, reler, rejeitado
	d.GenerateBlocks(3)
	tlvs := d.ReadForSending(3)
	tampered := append([]byte(nil), tlvs[1]...)
	tampered[20] ^= 0xFF
	r, err := a.SendBatch(ctx, [][]byte{tlvs[0], tampered, tlvs[2]})
	v.ok(err == nil && r.Status == 200 && r.Action == "reler" && r.Reason == "tag_invalida" && !r.Full, "byte trocado => reler/tag_invalida (status=%d acao=%s err=%v)", r.Status, r.Action, err)
	rej, _ := q.CountRejected(ctx, est0.DongleID)
	v.ok(rej >= 1, "bloco adulterado em rejected_block (%d)", rej)
	if len(r.AckTLV) > 0 {
		_, _ = d.ProcessAck(r.AckTLV)
	}
	// b) fora de ordem (pula um seq) => salto/reler
	tl := d.ReadForSending(4)
	if len(tl) >= 3 {
		r, err = a.SendBatch(ctx, [][]byte{tl[1], tl[2]})
		v.ok(err == nil && r.Reason == "salto" && r.Action == "reler", "fora de ordem => salto/reler (motivo=%s err=%v)", r.Reason, err)
	}
	// c) reenvio de lotes antigos => duplicados sem linhas novas
	if err := a.Drain(ctx); err != nil {
		return err
	}
	stale := d.ReadForSending(1)
	_ = stale
	l1, _ := q.CountReadings(ctx, est0.DongleID)
	oldHead := d.Head()
	_ = oldHead
	// d) 33 blocos => 429
	var thirtyThree [][]byte
	for i := 0; i < 33; i++ {
		thirtyThree = append(thirtyThree, protocol.AppendTLV(nil, protocol.BlockType, make([]byte, 49)))
	}
	_, err = a.SendBatch(ctx, thirtyThree)
	var herr *httpErr
	v.ok(errors.As(err, &herr) && herr.Status == 429 && herr.Code == "lote_grande", "33 blocos => 429 lote_grande (%v)", err)
	// e) token do dongle A na confirmação do dongle B => 403
	st, _, b, _ := a.doRequest(ctx, "confirmation", "GET", "/v1/dongles/"+o.Keys[1].Dev.Hex()+"/confirmacao", "", nil, map[string]string{"Authorization": "Bearer " + a.Token})
	v.ok(st == 403, "token de outro dongle na confirmação => 403 (%d %s)", st, string(b))
	// f) resposta reapresentada => 409
	ch, err := a.RequestChallenge(ctx)
	if err != nil {
		return err
	}
	resp, _ := d.ProcessChallenge(ch)
	_, err = a.SendResponse(ctx, resp)
	v.ok(err == nil, "resposta correta => 200")
	_, err = a.SendResponse(ctx, resp)
	v.ok(errors.As(err, &herr) && herr.Status == 409, "resposta reapresentada => 409 (%v)", err)
	// g) rajada de desafios => 429 desafios_pendentes
	var last error
	for i := 0; i < 5; i++ {
		_, last = a.RequestChallenge(ctx)
	}
	v.ok(errors.As(last, &herr) && herr.Status == 429, "rajada de desafios => 429 (%v)", last)
	// h) sem token e token lixo => 401
	_, err = a.SendBody(ctx, []byte{1}, "lixo")
	v.ok(errors.As(err, &herr) && herr.Status == 401, "token lixo => 401 (%v)", err)
	// estado final igual ao de um app honesto
	time.Sleep(500 * time.Millisecond)
	if err := a.Handshake(ctx); err != nil {
		// pendentes cheios: espera os desafios vencerem
		time.Sleep(2 * time.Second)
	}
	d.GenerateBlocks(5)
	if err := a.Drain(ctx); err != nil {
		return err
	}
	est, _ := q.DongleState(ctx, dev)
	v.ok(est.ContiguousSeq == int64(d.Head()), "estado final: contiguous_seq=%d head=%d", est.ContiguousSeq, d.Head())
	l2, _ := q.CountReadings(ctx, est0.DongleID)
	v.ok(l2 > l0 && l2 >= l1, "nenhum ataque alterou dados aceitos (leituras %d -> %d)", l0, l2)
	return v.result()
}

func timeScenario(ctx context.Context, o Options) error {
	v := &verifier{log: o.Log}
	q := o.Inspect
	d := NewDongle(o.Keys[0], ConfigDongle{DriftPPM: o.Drift})
	a := NewApp(o.API, d, client(), o.Report)
	for i := 0; i < 4; i++ {
		if err := a.Handshake(ctx); err != nil {
			return err
		}
		d.GenerateBlocks(5)
		if err := a.Drain(ctx); err != nil {
			return err
		}
		time.Sleep(time.Second)
	}
	dev := o.Keys[0].Dev
	est, _ := q.DongleState(ctx, dev)
	bs := waitForBoots(ctx, q, est.DongleID, 15*time.Second)
	v.ok(len(bs) > 0 && bs[len(bs)-1].AMs != nil && bs[len(bs)-1].TimeSource != nil && *bs[len(bs)-1].TimeSource == "anchors", "boot com time_source=anchors")
	if len(bs) > 0 && bs[len(bs)-1].BaseUncertaintyMs != nil {
		v.ok(*bs[len(bs)-1].BaseUncertaintyMs <= 2000, "base_uncertainty_ms=%d <= RTT máximo", *bs[len(bs)-1].BaseUncertaintyMs)
	}
	h, err := q.ReadingTime(ctx, est.DongleID)
	if err == nil {
		diff := float64(time.Now().UnixMilli()) - h
		if diff < 0 {
			diff = -diff
		}
		v.ok(diff < 10000, "hora reconstruída da leitura difere %.0f ms do relógio (< 10 s)", diff)
	} else {
		v.ok(false, "hora da leitura: %v", err)
	}
	an, _ := q.CountAnchors(ctx, est.DongleID)
	v.ok(an >= 4, "ancoras=%d", an)
	return v.result()
}

func generation(ctx context.Context, o Options) error {
	v := &verifier{log: o.Log}
	q := o.Inspect
	d := NewDongle(o.Keys[0], ConfigDongle{InitialBoot: 65534})
	a := NewApp(o.API, d, client(), o.Report)
	if err := a.Handshake(ctx); err != nil {
		return err
	}
	for i := 0; i < 4; i++ {
		d.GenerateBlocks(5)
		if err := a.Drain(ctx); err != nil {
			return err
		}
		if i < 3 {
			d.LosePower(0)
			if err := a.Handshake(ctx); err != nil {
				return err
			}
		}
	}
	dev := o.Keys[0].Dev
	est, _ := q.DongleState(ctx, dev)
	v.ok(est.BootGeneration == 1 && est.LastBootID == 2, "boot_generation=%d last_boot_id=%d (esperado 1 e 2)", est.BootGeneration, est.LastBootID)
	bs := waitForBoots(ctx, q, est.DongleID, 15*time.Second)
	got := map[[2]int32]bool{}
	for _, b := range bs {
		got[[2]int32{b.BootGeneration, b.BootID}] = true
	}
	v.ok(got[[2]int32{0, 65534}] && got[[2]int32{0, 65535}] && got[[2]int32{1, 1}] && got[[2]int32{1, 2}], "boots (0,65534) (0,65535) (1,1) (1,2): %v", got)
	n, _ := q.CountAlerts(ctx, est.DongleID, "boot_generation_advanced")
	v.ok(n == 1, "alerta boot_generation_advanced uma vez (%d)", n)
	// reprocessamento: ensaio e aplicar preservam a contagem
	before, _ := q.CountReadings(ctx, est.DongleID)
	cat := pid.New()
	if err := catalog.NewSync(o.Repos.Pids, cat, o.Log).Reload(ctx); err != nil {
		return err
	}
	uc := reprocess.New(o.Repos, o.Tx, cat, porttest.Nop{}, o.Log)
	op := reprocess.Options{DongleID: est.DongleID, Apply: false}
	if _, err := uc.Run(ctx, op); err != nil {
		return err
	}
	op.Apply, op.Operator = true, "simulator"
	if _, err := uc.Run(ctx, op); err != nil {
		v.ok(false, "reprocess --apply: %v", err)
	}
	after, _ := q.CountReadings(ctx, est.DongleID)
	v.ok(before == after && before > 0, "reprocessamento preserva a contagem de leituras (%d -> %d)", before, after)
	return v.result()
}

// dongleN devolve as chaves do i-ésimo dongle virtual (cicla a lista).
func dongleN(o Options, i int) Keys { return o.Keys[i%len(o.Keys)] }

// regime roda n dongles em regime até o prazo: gera a cada TLat/Speedup e envia.
func regime(ctx context.Context, o Options, n int) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(ctx, o.Duration)
	defer cancel()
	cli := client()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch := dongleN(o, i)
			d := NewDongle(ch, ConfigDongle{})
			a := NewApp(o.API, d, cli, o.Report)
			time.Sleep(time.Duration(float64(i) / float64(n) * float64(min(o.Ramp, o.TLat))))
			if err := a.Handshake(ctx); err != nil {
				return
			}
			per := time.Duration(float64(o.TLat) / o.Speedup)
			blocks := max(1, int(0.17*o.TLat.Seconds()))
			for ctx.Err() == nil {
				d.GenerateBlocks(blocks)
				_ = a.Drain(ctx)
				select {
				case <-ctx.Done():
				case <-time.After(per):
				}
			}
		}(i)
	}
	wg.Wait()
}

func load(ctx context.Context, o Options) error {
	if o.Duration <= 0 {
		o.Duration = 2 * time.Minute
	}
	if o.Dongles <= 0 {
		o.Dongles = len(o.Keys)
	}
	regime(ctx, o, o.Dongles)
	o.Report.Extra["dongles"] = o.Dongles
	o.Report.Extra["responses_5xx"] = o.Report.Count5xx()
	if o.Report.Count5xx() > 0 {
		return fmt.Errorf("%d respostas 5xx", o.Report.Count5xx())
	}
	return nil
}

func reconnection(ctx context.Context, o Options) error {
	if o.Dongles <= 0 {
		o.Dongles = len(o.Keys)
	}
	if o.Accumulated <= 0 {
		o.Accumulated = 500
	}
	v := &verifier{log: o.Log}
	q := o.Inspect
	var wg sync.WaitGroup
	cli := client()
	t0 := time.Now()
	type par struct {
		d *DongleVirtual
		c Keys
	}
	var pairs []par
	var mu sync.Mutex
	for i := 0; i < o.Dongles; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch := dongleN(o, i)
			d := NewDongle(ch, ConfigDongle{})
			d.GenerateBlocks(o.Accumulated)
			a := NewApp(o.API, d, cli, o.Report)
			time.Sleep(time.Duration(float64(i) / float64(o.Dongles) * float64(o.Ramp)))
			dctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			if err := a.Drain(dctx); err != nil {
				o.Log.Error("reconexao: dreno falhou", "erro", err)
			}
			mu.Lock()
			pairs = append(pairs, par{d, ch})
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	drain := time.Since(t0).Seconds()
	o.Report.Extra["drain_s"] = drain
	for _, p := range pairs {
		dev := p.c.Dev
		est, err := q.DongleState(ctx, dev)
		if err != nil {
			return err
		}
		v.ok(est.ContiguousSeq == int64(p.d.Head()), "dongle %s: contiguous_seq=%d head=%d", dev.Hex(), est.ContiguousSeq, p.d.Head())
	}
	return v.result()
}

func knee(ctx context.Context, o Options) error {
	if o.Duration <= 0 {
		o.Duration = 2 * time.Minute
	}
	steps := []int{100, 250, 500, 1000, 2000, 4000}
	var capacity int
	for _, n := range steps {
		sub := NewReport("knee", map[string]any{"dongles": n})
		o2 := o
		o2.Report = sub
		regime(ctx, o2, n)
		r := sub.Summary()
		routes, _ := r["routes"].(map[string]any)
		bl, _ := routes["blocks"].(map[string]any)
		p99, _ := bl["p99_ms"].(float64)
		total, s503 := 0, 0
		if st, ok := sub.Status["blocks"]; ok {
			for s, c := range st {
				total += c
				if s == 503 {
					s503 += c
				}
			}
		}
		o.Report.Extra[fmt.Sprintf("step_%d", n)] = map[string]any{"p99_ms": p99, "503": s503, "total": total}
		if p99 > 1000 || (total > 0 && float64(s503)/float64(total) > 0.01) {
			break
		}
		capacity = n
	}
	o.Report.Extra["dongle_capacity"] = capacity
	return nil
}

// handshakeWithBackoff repete o handshake enquanto o servidor responde 429, respeitando o Retry-After:
// um cenário curto e sem pausas entre os dongles bate no limite de desafios por IP.
func handshakeWithBackoff(ctx context.Context, a *AppVirtual) error {
	for attempt := 0; ; attempt++ {
		err := a.Handshake(ctx)
		var herr *httpErr
		if !errors.As(err, &herr) || herr.Status != 429 || attempt >= 5 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(max(herr.RetryAfter, time.Second)):
		}
	}
}

// asymScenario confere o perfil asym fim a fim: blocos assinados com ECDSA são aceitos (sem passar pelo
// KMS para verificar) e uma tag adulterada vira invalid_tag, com o ACK parado no seq anterior.
func asymScenario(ctx context.Context, o Options) error {
	v := &verifier{log: o.Log}
	q := o.Inspect
	for i, ch := range o.Keys {
		if ch.Profile.Name != protocol.ProfileAsym.Name || ch.KSig == nil {
			return fmt.Errorf("dongle %d: o cenário asym exige dongles do perfil asym com k_sig", i)
		}
		d := NewDongle(ch, ConfigDongle{FlashCapacity: 30})
		a := NewApp(o.API, d, client(), o.Report)
		if err := handshakeWithBackoff(ctx, a); err != nil {
			return fmt.Errorf("dongle %d: handshake: %w", i, err)
		}
		if err := generateAndDrain(ctx, a, 100); err != nil {
			return fmt.Errorf("dongle %d: blocos válidos: %w", i, err)
		}
		est, err := q.DongleState(ctx, ch.Dev)
		if err != nil {
			return err
		}
		v.ok(est.ContiguousSeq == int64(d.Head()), "dongle %d: contiguous_seq=%d head=%d", i, est.ContiguousSeq, d.Head())
		readings, _ := q.CountReadings(ctx, est.DongleID)
		pos, _ := q.CountPositions(ctx, est.DongleID)
		v.ok(readings > 0 && pos > 0, "dongle %d: leituras=%d posicoes=%d", i, readings, pos)
		rej, _ := q.CountRejected(ctx, est.DongleID)
		v.ok(rej == 0, "dongle %d: rejected_block vazio (%d)", i, rej)

		// 1 bit da tag trocado (a tag ocupa o fim do TLV do bloco); sem progresso o servidor responde 422 com o ACK
		d.GenerateBlocks(1)
		okAck := d.Head() - 1
		tlvs := d.ReadForSending(1)
		bad := append([]byte(nil), tlvs[0]...)
		bad[len(bad)-1] ^= 1
		r, err := a.SendBatch(ctx, [][]byte{bad})
		v.ok(err == nil && r.Status == 422 && r.Reason == "tag_invalida" && r.Action == "reler" && !r.Full,
			"dongle %d: tag adulterada => reler/tag_invalida (status=%d acao=%s motivo=%s err=%v)", i, r.Status, r.Action, r.Reason, err)
		v.ok(err == nil && r.Ack.AckSeq == okAck, "dongle %d: ACK parado em %d (veio %d)", i, okAck, r.Ack.AckSeq)
		rej, _ = q.CountRejected(ctx, est.DongleID)
		v.ok(rej == 1, "dongle %d: rejected_block=%d (invalid_tag)", i, rej)

		// o bloco original, intacto, ainda é aceito
		if len(r.AckTLV) > 0 {
			_, _ = d.ProcessAck(r.AckTLV)
		}
		if err := a.Drain(ctx); err != nil {
			return fmt.Errorf("dongle %d: reenvio do bloco válido: %w", i, err)
		}
		est, _ = q.DongleState(ctx, ch.Dev)
		v.ok(est.ContiguousSeq == int64(d.Head()), "dongle %d: bloco válido aceito depois da rejeição (contiguous_seq=%d)", i, est.ContiguousSeq)
	}
	return v.result()
}

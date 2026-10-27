package simulador

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/casodeuso/catalogo"
	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/repositorio/postgres"
	"tccelta/backend/internal/reprocessamento"
)

// Opcoes configuram a execução de um cenário.
type Opcoes struct {
	API       string
	Pool      *pgxpool.Pool // papel com SELECT (dono em dev/teste)
	Chaves    []Chaves
	Rel       *Relatorio
	Log       *slog.Logger
	Dongles   int
	Duracao   time.Duration
	TLat      time.Duration
	Acumulado int
	Rampa     time.Duration
	Acelerar  float64
	Deriva    float64
}

type verificador struct {
	falhas []string
	log    *slog.Logger
}

func (v *verificador) ok(cond bool, formato string, args ...any) {
	msg := fmt.Sprintf(formato, args...)
	if cond {
		v.log.Info("OK " + msg)
		fmt.Println("OK  ", msg)
		return
	}
	v.falhas = append(v.falhas, msg)
	fmt.Println("FALHOU", msg)
}

func (v *verificador) resultado() error {
	if len(v.falhas) > 0 {
		return fmt.Errorf("%d verificação(ões) falharam", len(v.falhas))
	}
	return nil
}

// Executar roda o cenário pelo nome.
func Executar(ctx context.Context, nome string, o Opcoes) error {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.TLat <= 0 {
		o.TLat = 60 * time.Second
	}
	if o.Acelerar <= 0 {
		o.Acelerar = 1
	}
	switch nome {
	case "e2e":
		return e2e(ctx, o)
	case "protocolo-negativo":
		return negativo(ctx, o)
	case "tempo":
		return cenarioTempo(ctx, o)
	case "geracao":
		return geracao(ctx, o)
	case "carga":
		return carga(ctx, o)
	case "reconexao":
		return reconexao(ctx, o)
	case "joelho":
		return joelho(ctx, o)
	}
	return fmt.Errorf("cenário desconhecido %q", nome)
}

func cliente() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 64, IdleConnTimeout: 90 * time.Second}}
}

// esperarBoots espera o worker fechar a fila de tempo do dongle.
func esperarBoots(ctx context.Context, q *consultas.Queries, id int32, maxEsperar time.Duration) []consultas.VerListarBootsRow {
	fim := time.Now().Add(maxEsperar)
	for {
		bs, _ := q.VerListarBoots(ctx, id)
		pend := false
		for _, b := range bs {
			pend = pend || b.PendenteTempo
		}
		if !pend || time.Now().After(fim) {
			return bs
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func cobertas(ls []consultas.VerListarLacunasRow, seq uint32) bool {
	for _, l := range ls {
		if int64(seq) >= l.FromSeq && int64(seq) <= l.ToSeq {
			return true
		}
	}
	return false
}

// drenarEm gera em lotes e drena, para que a flash pequena não descarte nada.
func gerarEDrenar(ctx context.Context, a *AppVirtual, n int) error {
	for n > 0 {
		c := min(n, 20)
		a.Dongle.GerarBlocos(c)
		n -= c
		if err := a.Drenar(ctx); err != nil {
			return err
		}
	}
	return nil
}

func e2e(ctx context.Context, o Opcoes) error {
	v := &verificador{log: o.Log}
	q := consultas.New(o.Pool)
	for i, ch := range o.Chaves {
		d := NovoDongle(ch, ConfigDongle{CapacidadeFlash: 30})
		a := NovoApp(o.API, d, cliente(), o.Rel)
		if err := a.Handshake(ctx); err != nil {
			return fmt.Errorf("dongle %d: handshake: %w", i, err)
		}
		if err := gerarEDrenar(ctx, a, 200); err != nil {
			return fmt.Errorf("dongle %d: fase 1: %w", i, err)
		}
		// queda de energia (BOOT com furo) e 50 blocos
		d.PerderEnergia(3)
		if err := a.Handshake(ctx); err != nil {
			return err
		}
		if err := gerarEDrenar(ctx, a, 50); err != nil {
			return fmt.Errorf("dongle %d: fase 2: %w", i, err)
		}
		// bloco corrompido (LOSS)
		d.GerarBlocos(10)
		d.Corromper(d.Head() - 5)
		if err := a.Drenar(ctx); err != nil {
			return fmt.Errorf("dongle %d: fase 3: %w", i, err)
		}
		// 5 min offline com flash pequena (OVERFLOW)
		d.GerarBlocos(60)
		if err := a.Handshake(ctx); err != nil {
			return err
		}
		if err := a.Drenar(ctx); err != nil {
			return fmt.Errorf("dongle %d: fase 4: %w", i, err)
		}
		if i == len(o.Chaves)-1 {
			// OVERFLOW que engole o segmento antes do BOOT (caso 16)
			d.GerarBlocos(20)
			d.PerderEnergia(5)
			d.GerarBlocos(4)
			d.DescartarSegmento(20)
			if err := a.Handshake(ctx); err != nil {
				return err
			}
			if err := a.Drenar(ctx); err != nil {
				return fmt.Errorf("dongle %d: caso 16: %w", i, err)
			}
			// BOOT corrompido (caso 18)
			d.PerderEnergia(3)
			d.GerarBlocos(2)
			d.Corromper(d.UltimoSeqBoot())
			if err := a.Handshake(ctx); err != nil {
				return err
			}
			if err := a.Drenar(ctx); err != nil {
				return fmt.Errorf("dongle %d: caso 18: %w", i, err)
			}
		}
		dev := ch.Dev
		est, err := q.VerEstadoDongle(ctx, dev[:])
		if err != nil {
			return err
		}
		head := d.Head()
		v.ok(est.SeqContiguo == int64(head) && est.HeadSeq == int64(head), "dongle %d: seq_contiguo=%d head=%d (dongle=%d)", i, est.SeqContiguo, est.HeadSeq, head)
		ls, _ := q.VerListarLacunas(ctx, est.DongleID)
		causas := map[string]int{}
		for _, l := range ls {
			causas[l.Causa]++
		}
		v.ok(causas["BOOT"] > 0 && causas["LOSS"] > 0 && causas["OVERFLOW"] > 0, "dongle %d: lacunas %v", i, causas)
		var esperado, espPos int64
		for seq, n := range d.Registros {
			if !cobertas(ls, seq) {
				esperado += int64(n - 1)
				espPos++
			}
		}
		n, _ := q.VerContarLeituras(ctx, est.DongleID)
		pos, _ := q.VerContarPosicoes(ctx, est.DongleID)
		v.ok(n > 0 && n == esperado && pos == espPos, "dongle %d: leituras=%d/%d posicoes=%d/%d", i, n, esperado, pos, espPos)
		rej, _ := q.VerContarRejeitados(ctx, est.DongleID)
		v.ok(rej == 0, "dongle %d: bloco_rejeitado vazio (%d)", i, rej)
		anc, _ := q.VerContarAncoras(ctx, est.DongleID)
		v.ok(anc >= 1, "dongle %d: ancoras=%d", i, anc)
		bs := esperarBoots(ctx, q, est.DongleID, 15*time.Second)
		comTempo := 0
		for _, b := range bs {
			if b.AMs != nil && b.OrigemTempo != nil {
				comTempo++
			}
		}
		v.ok(len(bs) > 0 && comTempo > 0, "dongle %d: boots=%d com tempo=%d", i, len(bs), comTempo)
	}
	return v.resultado()
}

func negativo(ctx context.Context, o Opcoes) error {
	v := &verificador{log: o.Log}
	q := consultas.New(o.Pool)
	if len(o.Chaves) < 2 {
		return errors.New("precisa de 2 dongles")
	}
	d := NovoDongle(o.Chaves[0], ConfigDongle{})
	a := NovoApp(o.API, d, cliente(), o.Rel)
	if err := a.Handshake(ctx); err != nil {
		return err
	}
	d.GerarBlocos(10)
	if err := a.Drenar(ctx); err != nil {
		return err
	}
	dev := o.Chaves[0].Dev
	est0, _ := q.VerEstadoDongle(ctx, dev[:])
	l0, _ := q.VerContarLeituras(ctx, est0.DongleID)

	// a) 1 byte trocado no 2º bloco => 200 interrompido, reler, rejeitado
	d.GerarBlocos(3)
	tlvs := d.LerParaEnvio(3)
	adulterado := append([]byte(nil), tlvs[1]...)
	adulterado[20] ^= 0xFF
	r, err := a.EnviarLote(ctx, [][]byte{tlvs[0], adulterado, tlvs[2]})
	v.ok(err == nil && r.Status == 200 && r.Acao == "reler" && r.Motivo == "tag_invalida" && !r.Completo, "byte trocado => reler/tag_invalida (status=%d acao=%s err=%v)", r.Status, r.Acao, err)
	rej, _ := q.VerContarRejeitados(ctx, est0.DongleID)
	v.ok(rej >= 1, "bloco adulterado em bloco_rejeitado (%d)", rej)
	if len(r.AckTLV) > 0 {
		_, _ = d.ProcessarAck(r.AckTLV)
	}
	// b) fora de ordem (pula um seq) => salto/reler
	tl := d.LerParaEnvio(4)
	if len(tl) >= 3 {
		r, err = a.EnviarLote(ctx, [][]byte{tl[1], tl[2]})
		v.ok(err == nil && r.Motivo == "salto" && r.Acao == "reler", "fora de ordem => salto/reler (motivo=%s err=%v)", r.Motivo, err)
	}
	// c) reenvio de lotes antigos => duplicados sem linhas novas
	if err := a.Drenar(ctx); err != nil {
		return err
	}
	antigos := d.LerParaEnvio(1)
	_ = antigos
	l1, _ := q.VerContarLeituras(ctx, est0.DongleID)
	vel := d.Head()
	_ = vel
	// d) 33 blocos => 429
	var trinta3 [][]byte
	for i := 0; i < 33; i++ {
		trinta3 = append(trinta3, protocolo.AnexarTLV(nil, protocolo.TipoBloco, make([]byte, 49)))
	}
	_, err = a.EnviarLote(ctx, trinta3)
	var eh *erroHTTP
	v.ok(errors.As(err, &eh) && eh.Status == 429 && eh.Codigo == "lote_grande", "33 blocos => 429 lote_grande (%v)", err)
	// e) token do dongle A na confirmação do dongle B => 403
	st, _, b, _ := a.fazer(ctx, "confirmacao", "GET", "/v1/dongles/"+o.Chaves[1].Dev.Hex()+"/confirmacao", "", nil, map[string]string{"Authorization": "Bearer " + a.Token})
	v.ok(st == 403, "token de outro dongle na confirmação => 403 (%d %s)", st, string(b))
	// f) resposta reapresentada => 409
	des, err := a.PedirDesafio(ctx)
	if err != nil {
		return err
	}
	resp, _ := d.ProcessarDesafio(des)
	_, err = a.EnviarResposta(ctx, resp)
	v.ok(err == nil, "resposta correta => 200")
	_, err = a.EnviarResposta(ctx, resp)
	v.ok(errors.As(err, &eh) && eh.Status == 409, "resposta reapresentada => 409 (%v)", err)
	// g) rajada de desafios => 429 desafios_pendentes
	var ultimo error
	for i := 0; i < 5; i++ {
		_, ultimo = a.PedirDesafio(ctx)
	}
	v.ok(errors.As(ultimo, &eh) && eh.Status == 429, "rajada de desafios => 429 (%v)", ultimo)
	// h) sem token e token lixo => 401
	_, err = a.EnviarCorpo(ctx, []byte{1}, "lixo")
	v.ok(errors.As(err, &eh) && eh.Status == 401, "token lixo => 401 (%v)", err)
	// estado final igual ao de um app honesto
	time.Sleep(500 * time.Millisecond)
	if err := a.Handshake(ctx); err != nil {
		// pendentes cheios: espera os desafios vencerem
		time.Sleep(2 * time.Second)
	}
	d.GerarBlocos(5)
	if err := a.Drenar(ctx); err != nil {
		return err
	}
	est, _ := q.VerEstadoDongle(ctx, dev[:])
	v.ok(est.SeqContiguo == int64(d.Head()), "estado final: seq_contiguo=%d head=%d", est.SeqContiguo, d.Head())
	l2, _ := q.VerContarLeituras(ctx, est0.DongleID)
	v.ok(l2 > l0 && l2 >= l1, "nenhum ataque alterou dados aceitos (leituras %d -> %d)", l0, l2)
	return v.resultado()
}

func cenarioTempo(ctx context.Context, o Opcoes) error {
	v := &verificador{log: o.Log}
	q := consultas.New(o.Pool)
	d := NovoDongle(o.Chaves[0], ConfigDongle{DerivaPPM: o.Deriva})
	a := NovoApp(o.API, d, cliente(), o.Rel)
	for i := 0; i < 4; i++ {
		if err := a.Handshake(ctx); err != nil {
			return err
		}
		d.GerarBlocos(5)
		if err := a.Drenar(ctx); err != nil {
			return err
		}
		time.Sleep(time.Second)
	}
	dev := o.Chaves[0].Dev
	est, _ := q.VerEstadoDongle(ctx, dev[:])
	bs := esperarBoots(ctx, q, est.DongleID, 15*time.Second)
	v.ok(len(bs) > 0 && bs[len(bs)-1].AMs != nil && bs[len(bs)-1].OrigemTempo != nil && *bs[len(bs)-1].OrigemTempo == "ancoras", "boot com origem_tempo=ancoras")
	if len(bs) > 0 && bs[len(bs)-1].IncertezaBaseMs != nil {
		v.ok(*bs[len(bs)-1].IncertezaBaseMs <= 2000, "incerteza_base_ms=%d <= RTT máximo", *bs[len(bs)-1].IncertezaBaseMs)
	}
	h, err := q.VerHoraDaLeitura(ctx, est.DongleID)
	if err == nil {
		dif := float64(time.Now().UnixMilli()) - h
		if dif < 0 {
			dif = -dif
		}
		v.ok(dif < 10000, "hora reconstruída da leitura difere %.0f ms do relógio (< 10 s)", dif)
	} else {
		v.ok(false, "hora da leitura: %v", err)
	}
	an, _ := q.VerContarAncoras(ctx, est.DongleID)
	v.ok(an >= 4, "ancoras=%d", an)
	return v.resultado()
}

func geracao(ctx context.Context, o Opcoes) error {
	v := &verificador{log: o.Log}
	q := consultas.New(o.Pool)
	d := NovoDongle(o.Chaves[0], ConfigDongle{BootInicial: 65534})
	a := NovoApp(o.API, d, cliente(), o.Rel)
	if err := a.Handshake(ctx); err != nil {
		return err
	}
	for i := 0; i < 4; i++ {
		d.GerarBlocos(5)
		if err := a.Drenar(ctx); err != nil {
			return err
		}
		if i < 3 {
			d.PerderEnergia(0)
			if err := a.Handshake(ctx); err != nil {
				return err
			}
		}
	}
	dev := o.Chaves[0].Dev
	est, _ := q.VerEstadoDongle(ctx, dev[:])
	v.ok(est.GeracaoBoot == 1 && est.UltimoBootID == 2, "geracao_boot=%d ultimo_boot_id=%d (esperado 1 e 2)", est.GeracaoBoot, est.UltimoBootID)
	bs := esperarBoots(ctx, q, est.DongleID, 15*time.Second)
	got := map[[2]int32]bool{}
	for _, b := range bs {
		got[[2]int32{b.GeracaoBoot, b.BootID}] = true
	}
	v.ok(got[[2]int32{0, 65534}] && got[[2]int32{0, 65535}] && got[[2]int32{1, 1}] && got[[2]int32{1, 2}], "boots (0,65534) (0,65535) (1,1) (1,2): %v", got)
	n, _ := q.VerContarAlertas(ctx, est.DongleID, "geracao_boot_avancou")
	v.ok(n == 1, "alerta geracao_boot_avancou uma vez (%d)", n)
	// reprocessamento: ensaio e aplicar preservam a contagem
	antes, _ := q.VerContarLeituras(ctx, est.DongleID)
	cat := pid.Novo()
	if err := catalogo.NovoSincronizar(postgres.Repositorios(q).Pids, cat, o.Log).Recarregar(ctx); err != nil {
		return err
	}
	op := reprocessamento.OpcoesReprocesso{DongleID: est.DongleID, Aplicar: false}
	if _, err := reprocessamento.Reprocessar(ctx, o.Pool, cat, op, o.Log, nil); err != nil {
		return err
	}
	op.Aplicar, op.Operador = true, "simulador"
	if _, err := reprocessamento.Reprocessar(ctx, o.Pool, cat, op, o.Log, nil); err != nil {
		v.ok(false, "reprocessar --aplicar: %v", err)
	}
	depois, _ := q.VerContarLeituras(ctx, est.DongleID)
	v.ok(antes == depois && antes > 0, "reprocessamento preserva a contagem de leituras (%d -> %d)", antes, depois)
	return v.resultado()
}

// dongleN devolve as chaves do i-ésimo dongle virtual (cicla a lista).
func dongleN(o Opcoes, i int) Chaves { return o.Chaves[i%len(o.Chaves)] }

// regime roda n dongles em regime até o prazo: gera a cada TLat/Acelerar e envia.
func regime(ctx context.Context, o Opcoes, n int) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(ctx, o.Duracao)
	defer cancel()
	cli := cliente()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch := dongleN(o, i)
			d := NovoDongle(ch, ConfigDongle{})
			a := NovoApp(o.API, d, cli, o.Rel)
			time.Sleep(time.Duration(float64(i) / float64(n) * float64(min(o.Rampa, o.TLat))))
			if err := a.Handshake(ctx); err != nil {
				return
			}
			per := time.Duration(float64(o.TLat) / o.Acelerar)
			blocos := max(1, int(0.17*o.TLat.Seconds()))
			for ctx.Err() == nil {
				d.GerarBlocos(blocos)
				_ = a.Drenar(ctx)
				select {
				case <-ctx.Done():
				case <-time.After(per):
				}
			}
		}(i)
	}
	wg.Wait()
}

func carga(ctx context.Context, o Opcoes) error {
	if o.Duracao <= 0 {
		o.Duracao = 2 * time.Minute
	}
	if o.Dongles <= 0 {
		o.Dongles = len(o.Chaves)
	}
	regime(ctx, o, o.Dongles)
	o.Rel.Extras["dongles"] = o.Dongles
	o.Rel.Extras["respostas_5xx"] = o.Rel.Cinco()
	if o.Rel.Cinco() > 0 {
		return fmt.Errorf("%d respostas 5xx", o.Rel.Cinco())
	}
	return nil
}

func reconexao(ctx context.Context, o Opcoes) error {
	if o.Dongles <= 0 {
		o.Dongles = len(o.Chaves)
	}
	if o.Acumulado <= 0 {
		o.Acumulado = 500
	}
	v := &verificador{log: o.Log}
	q := consultas.New(o.Pool)
	var wg sync.WaitGroup
	cli := cliente()
	t0 := time.Now()
	type par struct {
		d *DongleVirtual
		c Chaves
	}
	var pares []par
	var mu sync.Mutex
	for i := 0; i < o.Dongles; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch := dongleN(o, i)
			d := NovoDongle(ch, ConfigDongle{})
			d.GerarBlocos(o.Acumulado)
			a := NovoApp(o.API, d, cli, o.Rel)
			time.Sleep(time.Duration(float64(i) / float64(o.Dongles) * float64(o.Rampa)))
			dctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			if err := a.Drenar(dctx); err != nil {
				o.Log.Error("reconexao: dreno falhou", "erro", err)
			}
			mu.Lock()
			pares = append(pares, par{d, ch})
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	dren := time.Since(t0).Seconds()
	o.Rel.Extras["drenagem_s"] = dren
	for _, p := range pares {
		dev := p.c.Dev
		est, err := q.VerEstadoDongle(ctx, dev[:])
		if err != nil {
			return err
		}
		v.ok(est.SeqContiguo == int64(p.d.Head()), "dongle %s: seq_contiguo=%d head=%d", dev.Hex(), est.SeqContiguo, p.d.Head())
	}
	return v.resultado()
}

func joelho(ctx context.Context, o Opcoes) error {
	if o.Duracao <= 0 {
		o.Duracao = 2 * time.Minute
	}
	degraus := []int{100, 250, 500, 1000, 2000, 4000}
	var capacidade int
	for _, n := range degraus {
		sub := NovoRelatorio("joelho", map[string]any{"dongles": n})
		o2 := o
		o2.Rel = sub
		regime(ctx, o2, n)
		r := sub.Resumo()
		rotas, _ := r["rotas"].(map[string]any)
		bl, _ := rotas["blocos"].(map[string]any)
		p99, _ := bl["p99_ms"].(float64)
		total, s503 := 0, 0
		if st, ok := sub.Status["blocos"]; ok {
			for s, c := range st {
				total += c
				if s == 503 {
					s503 += c
				}
			}
		}
		o.Rel.Extras[fmt.Sprintf("degrau_%d", n)] = map[string]any{"p99_ms": p99, "503": s503, "total": total}
		if p99 > 1000 || (total > 0 && float64(s503)/float64(total) > 0.01) {
			break
		}
		capacidade = n
	}
	o.Rel.Extras["capacidade_dongles"] = capacidade
	return nil
}

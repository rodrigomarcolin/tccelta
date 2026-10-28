// Package memoria implementa os repositórios em memória, só para testes dos casos de uso e do HTTP.
// Não há locking nos repositórios: EmTransacao serializa as transações com um mutex e desfaz as
// escritas se fn devolver erro; fora de transação, os testes são sequenciais.
package memoria

import (
	"bytes"
	"context"
	"sync"
	"time"

	"tccelta/backend/internal/dominio/dongle"
	"tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/ocorrencia"
	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/dominio/tempo"
	"tccelta/backend/internal/repositorio"
)

type chaveDesafio struct {
	dongle int32
	cs     int64
}

// Desafio é um desafio guardado.
type Desafio struct {
	handshake.DesafioGravado
	Resultado string
	Final     handshake.Finalizacao
}

type chaveBoot struct{ dongle, geracao, boot int32 }

// Banco é o estado em memória; tudo que é exportado serve para os testes conferirem o resultado.
type Banco struct {
	mu sync.Mutex

	dongles  map[protocolo.DevID]dongle.Dongle
	estados  map[int32]dongle.Estado
	ultimoCS map[int32]int64
	desafios map[chaveDesafio]Desafio
	tmsBoot  map[chaveBoot]int64
	vistos   map[[2]int64]bool // (dongle, seq) já gravados em bloco_bruto
	Defs     []pid.Def

	Brutos       []ingestao.BlocoBruto
	Rejeitados   []ingestao.Rejeitado
	Leituras     []pid.Leitura
	Posicoes     []pid.Posicao
	Lacunas      []ocorrencia.Lacuna
	Alertas      []ocorrencia.Alerta
	Ancoras      []handshake.Ancora
	Notificacoes []string
}

// Novo cria o banco vazio.
func Novo() *Banco {
	return &Banco{dongles: map[protocolo.DevID]dongle.Dongle{}, estados: map[int32]dongle.Estado{}, ultimoCS: map[int32]int64{},
		desafios: map[chaveDesafio]Desafio{}, tmsBoot: map[chaveBoot]int64{}, vistos: map[[2]int64]bool{}}
}

// AdicionarDongle cadastra um dongle (e seu estado zerado).
func (b *Banco) AdicionarDongle(d dongle.Dongle) {
	b.dongles[d.DevID] = d
	b.estados[d.ID] = dongle.Estado{}
}

// DefinirEstado sobrescreve o estado do dongle.
func (b *Banco) DefinirEstado(id int32, e dongle.Estado) { b.estados[id] = e }

// Estado devolve o estado atual do dongle.
func (b *Banco) Estado(id int32) dongle.Estado { return b.estados[id] }

// Desafio devolve o desafio (dongle, c_S).
func (b *Banco) Desafio(id int32, cs int64) (Desafio, bool) {
	d, ok := b.desafios[chaveDesafio{id, cs}]
	return d, ok
}

// Repositorios devolve os repositórios sobre o banco.
func (b *Banco) Repositorios() repositorio.Repositorios {
	return repositorio.Repositorios{Dongles: (*dongles)(b), Desafios: (*desafios)(b), Estados: (*estados)(b),
		Boots: (*boots)(b), Blocos: (*blocos)(b), Ocorrencias: (*ocorrencias)(b), Pids: (*pids)(b)}
}

// EmTransacao implementa repositorio.Transacionador.
func (b *Banco) EmTransacao(ctx context.Context, fn func(ctx context.Context, r repositorio.Repositorios) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	salvo := b.copiar()
	if err := fn(ctx, b.Repositorios()); err != nil {
		b.restaurar(salvo)
		return err
	}
	return nil
}

func (b *Banco) copiar() *Banco {
	c := Novo()
	for k, v := range b.dongles {
		c.dongles[k] = v
	}
	for k, v := range b.estados {
		c.estados[k] = v
	}
	for k, v := range b.ultimoCS {
		c.ultimoCS[k] = v
	}
	for k, v := range b.desafios {
		c.desafios[k] = v
	}
	for k, v := range b.tmsBoot {
		c.tmsBoot[k] = v
	}
	for k, v := range b.vistos {
		c.vistos[k] = v
	}
	c.Brutos, c.Rejeitados, c.Leituras, c.Posicoes = b.Brutos[:len(b.Brutos):len(b.Brutos)], b.Rejeitados[:len(b.Rejeitados):len(b.Rejeitados)],
		b.Leituras[:len(b.Leituras):len(b.Leituras)], b.Posicoes[:len(b.Posicoes):len(b.Posicoes)]
	c.Lacunas, c.Alertas = b.Lacunas[:len(b.Lacunas):len(b.Lacunas)], b.Alertas[:len(b.Alertas):len(b.Alertas)]
	c.Ancoras, c.Notificacoes = b.Ancoras[:len(b.Ancoras):len(b.Ancoras)], b.Notificacoes[:len(b.Notificacoes):len(b.Notificacoes)]
	return c
}

func (b *Banco) restaurar(c *Banco) {
	b.dongles, b.estados, b.ultimoCS, b.desafios, b.tmsBoot, b.vistos = c.dongles, c.estados, c.ultimoCS, c.desafios, c.tmsBoot, c.vistos
	b.Brutos, b.Rejeitados, b.Leituras, b.Posicoes = c.Brutos, c.Rejeitados, c.Leituras, c.Posicoes
	b.Lacunas, b.Alertas, b.Ancoras, b.Notificacoes = c.Lacunas, c.Alertas, c.Ancoras, c.Notificacoes
}

type dongles Banco

func (r *dongles) BuscarPorDevID(_ context.Context, dev protocolo.DevID) (dongle.Dongle, error) {
	d, ok := r.dongles[dev]
	if !ok {
		return dongle.Dongle{}, dongle.ErrDesconhecido
	}
	return d, nil
}
func (r *dongles) Travar(context.Context, int32) error { return nil }

type desafios Banco

func (r *desafios) Emitir(_ context.Context, e handshake.Emissao) (handshake.Desafio, error) {
	pend := 0
	for k, d := range r.desafios {
		if k.dongle == e.DongleID && d.RespondidoEm == nil && d.ExpiraEm.After(e.S1) {
			pend++
		}
	}
	if pend >= e.MaxPendentes {
		return handshake.Desafio{}, handshake.ErrDesafiosPendentes
	}
	r.ultimoCS[e.DongleID]++
	cs := r.ultimoCS[e.DongleID]
	r.desafios[chaveDesafio{e.DongleID, cs}] = Desafio{DesafioGravado: handshake.DesafioGravado{Nonce: e.Nonce, S1: e.S1, ExpiraEm: e.ExpiraEm}}
	est := r.estados[e.DongleID]
	return handshake.Desafio{CS: cs, SeqContiguo: est.SeqContiguo, CConf: est.CConf}, nil
}

func (r *desafios) Buscar(_ context.Context, id int32, cs int64) (handshake.DesafioGravado, error) {
	d, ok := r.desafios[chaveDesafio{id, cs}]
	if !ok {
		return handshake.DesafioGravado{}, handshake.ErrDesafioInvalido
	}
	return d.DesafioGravado, nil
}

func (r *desafios) Consumir(_ context.Context, c handshake.Consumo) (time.Time, error) {
	k := chaveDesafio{c.DongleID, c.CS}
	d, ok := r.desafios[k]
	if !ok || !bytes.Equal(d.Nonce, c.Nonce) || d.RespondidoEm != nil || !d.ExpiraEm.After(c.S2) {
		return time.Time{}, handshake.ErrDesafioInvalido
	}
	s2 := c.S2
	d.RespondidoEm = &s2
	r.desafios[k] = d
	return d.S1, nil
}

func (r *desafios) Finalizar(_ context.Context, f handshake.Finalizacao) error {
	k := chaveDesafio{f.DongleID, f.CS}
	d := r.desafios[k]
	d.Resultado, d.Final = f.Resultado, f
	r.desafios[k] = d
	return nil
}

func (r *desafios) MarcarTagInvalida(_ context.Context, id int32, cs int64, nonce []byte) error {
	k := chaveDesafio{id, cs}
	if d, ok := r.desafios[k]; ok && bytes.Equal(d.Nonce, nonce) {
		d.Resultado = "tag_invalida"
		r.desafios[k] = d
	}
	return nil
}

type estados Banco

func (r *estados) Ler(_ context.Context, id int32) (dongle.Estado, error) {
	e := r.estados[id]
	e.UltimoAck = nil
	return e, nil
}

func (r *estados) LerSnapshot(_ context.Context, id int32) (dongle.Estado, error) {
	e := r.estados[id]
	return dongle.Estado{SeqContiguo: e.SeqContiguo, CConf: e.CConf, UltimoAck: e.UltimoAck}, nil
}

func (r *estados) LerUltimoAck(_ context.Context, id int32) ([]byte, error) {
	return r.estados[id].UltimoAck, nil
}

func (r *estados) AtualizarHandshake(_ context.Context, a handshake.Atualizacao) error {
	e := r.estados[a.DongleID]
	e.SeqContiguo, e.MaiorSeqVisto, e.HeadSeq, e.CConf, e.UltimoAck = a.SeqContiguo, max(e.MaiorSeqVisto, a.SeqContiguo), a.HeadSeq, a.CConf, a.UltimoAck
	r.estados[a.DongleID] = e
	return nil
}

func (r *estados) AtualizarIngestao(_ context.Context, a ingestao.AtualizacaoEstado) error {
	e := r.estados[a.DongleID]
	e.SeqContiguo, e.MaiorSeqVisto, e.UltimoAck = a.SeqContiguo, max(e.MaiorSeqVisto, a.MaiorVisto, a.SeqContiguo), a.UltimoAck
	e.GeracaoBoot, e.UltimoBootID = a.GeracaoBoot, a.UltimoBootID
	r.estados[a.DongleID] = e
	return nil
}

type boots Banco

func (r *boots) TMsUltimo(_ context.Context, id int32, bootIDs []int32, gMin, gMax int32) ([]tempo.RefBoot, error) {
	var out []tempo.RefBoot
	for _, bid := range bootIDs {
		for g := gMin; g <= gMax; g++ {
			if t, ok := r.tmsBoot[chaveBoot{id, g, bid}]; ok {
				out = append(out, tempo.RefBoot{Geracao: g, BootID: bid, TMsUltimo: t})
			}
		}
	}
	return out, nil
}

func (r *boots) GarantirPendente(context.Context, int32, int32, int32, time.Time) error { return nil }

func (r *boots) AtualizarIngestao(_ context.Context, id int32, _ time.Time, ags []ingestao.AgregadoBoot) error {
	for _, a := range ags {
		k := chaveBoot{id, a.Geracao, a.BootID}
		r.tmsBoot[k] = max(r.tmsBoot[k], a.TFim)
	}
	return nil
}

func (r *boots) InserirAncora(_ context.Context, a handshake.Ancora) error {
	r.Ancoras = append(r.Ancoras, a)
	return nil
}

func (r *boots) NotificarPendente(_ context.Context, chave string) error {
	r.Notificacoes = append(r.Notificacoes, chave)
	return nil
}

type blocos Banco

func (r *blocos) InserirBrutos(_ context.Context, id int32, _ int64, _ time.Time, bs []ingestao.BlocoBruto) (int, error) {
	n := 0
	for _, x := range bs {
		k := [2]int64{int64(id), x.Seq}
		if r.vistos[k] {
			continue
		}
		r.vistos[k] = true
		r.Brutos = append(r.Brutos, x)
		n++
	}
	return n, nil
}

func (r *blocos) RegistrarRejeitado(_ context.Context, x ingestao.Rejeitado) error {
	r.Rejeitados = append(r.Rejeitados, x)
	return nil
}

func (r *blocos) CopiarLeituras(_ context.Context, _ int32, _ time.Time, ls []pid.Leitura) error {
	r.Leituras = append(r.Leituras, ls...)
	return nil
}

func (r *blocos) CopiarPosicoes(_ context.Context, _ int32, _ time.Time, ps []pid.Posicao) error {
	r.Posicoes = append(r.Posicoes, ps...)
	return nil
}

type ocorrencias Banco

func (r *ocorrencias) InserirLacuna(_ context.Context, l ocorrencia.Lacuna) error {
	r.Lacunas = append(r.Lacunas, l)
	return nil
}

func (r *ocorrencias) InserirAlerta(_ context.Context, a ocorrencia.Alerta) error {
	r.Alertas = append(r.Alertas, a)
	return nil
}

type pids Banco

func (r *pids) Listar(context.Context) ([]pid.Def, error) { return r.Defs, nil }

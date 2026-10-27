// Package handshake tem os casos de uso do desafio e da resposta (spec 04).
package handshake

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"time"

	"tccelta/backend/internal/casodeuso/porta"
	dhs "tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/observabilidade"
	"tccelta/backend/internal/repositorio"
)

// Parametros são os ajustes do handshake.
type Parametros struct {
	DesafioValidade    time.Duration
	MaxPendentes       int
	AncoraRTTMax       time.Duration
	TLatSugerido       time.Duration
	PermitirPerfilNone bool
}

// EmitirDesafioEntrada identifica o dongle que pede o desafio.
type EmitirDesafioEntrada struct{ DevID protocolo.DevID }

// DesafioEmitido é a saída de EmitirDesafio.
type DesafioEmitido struct {
	TLV      []byte
	ExpiraEm time.Time
	// RetryAfter é preenchido junto com dhs.ErrDesafiosPendentes.
	RetryAfter time.Duration
}

// EmitirDesafio gera o Desafio 0x01 assinado com K_auth (POST /v1/dongles/{id}/desafio).
type EmitirDesafio struct {
	repos repositorio.Repositorios
	kms   porta.Autenticador
	met   porta.MetricasHandshake
	agora porta.Relogio
	rand  io.Reader
	log   *slog.Logger
	p     Parametros
}

// NovoEmitirDesafio liga o caso de uso às suas portas.
func NovoEmitirDesafio(repos repositorio.Repositorios, kms porta.Autenticador, met porta.MetricasHandshake,
	agora porta.Relogio, log *slog.Logger, p Parametros) *EmitirDesafio {
	return &EmitirDesafio{repos: repos, kms: kms, met: met, agora: agora, rand: rand.Reader, log: log, p: p}
}

func tag16(mac []byte) []byte { return mac[:protocolo.TamTagAuth] }

// Executar emite o desafio.
func (c *EmitirDesafio) Executar(ctx context.Context, e EmitirDesafioEntrada) (DesafioEmitido, error) {
	d, err := c.repos.Dongles.BuscarPorDevID(ctx, e.DevID)
	if err != nil {
		return DesafioEmitido{}, err
	}
	if err := d.VerificarAcesso(c.p.PermitirPerfilNone); err != nil {
		return DesafioEmitido{}, err
	}
	var ns [16]byte
	if _, err := io.ReadFull(c.rand, ns[:]); err != nil {
		return DesafioEmitido{}, err
	}
	s1 := dhs.TruncarMicro(c.agora())
	expira := s1.Add(c.p.DesafioValidade)
	des, err := c.repos.Desafios.Emitir(ctx, dhs.Emissao{DongleID: d.ID, S1: s1, ExpiraEm: expira, Nonce: ns[:],
		MaxPendentes: c.p.MaxPendentes})
	if err != nil {
		if errors.Is(err, dhs.ErrDesafiosPendentes) {
			return DesafioEmitido{RetryAfter: c.pendenteMaisAntigo()}, err
		}
		return DesafioEmitido{}, err
	}
	msg := protocolo.Desafio{CS: uint32(des.CS), NS: ns, AckSeq: uint32(des.SeqContiguo), CConf: uint32(des.CConf)}
	macs, err := c.kms.HMACAuth(observabilidade.ComFaseKMS(ctx, observabilidade.FasePre), d.ChaveAuth,
		[][]byte{protocolo.EntradaTag(protocolo.TipoDesafio, d.DevID, msg.Payload())})
	if err != nil {
		return DesafioEmitido{}, err
	}
	tlv, err := msg.Montar(tag16(macs[0]))
	if err != nil {
		return DesafioEmitido{}, err
	}
	c.log.Info("handshake.desafio_emitido", "dongle", d.DevID.Hex(), "c_s", msg.CS, "ack_seq", msg.AckSeq)
	c.met.Handshake("desafio_emitido")
	return DesafioEmitido{TLV: tlv, ExpiraEm: expira}, nil
}

// pendenteMaisAntigo: sem consulta dedicada, usa um terço da validade do desafio como teto conservador
// (mínimo 1 s) para o Retry-After.
func (c *EmitirDesafio) pendenteMaisAntigo() time.Duration {
	return time.Second * time.Duration(max(1, int(c.p.DesafioValidade.Seconds())/3))
}

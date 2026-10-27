package handshake

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/dominio/dongle"
	dhs "tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/ocorrencia"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/dominio/tempo"
	"tccelta/backend/internal/observabilidade"
	"tccelta/backend/internal/repositorio"
)

// ResponderDesafioEntrada é a resposta do dongle. S2 é o instante de chegada da requisição.
type ResponderDesafioEntrada struct {
	DevID protocolo.DevID
	S2    time.Time
	TLV   []byte
}

// ResultadoHandshake é a saída de ResponderDesafio.
type ResultadoHandshake struct {
	AckTLV        []byte
	Token         string
	TokenExpiraEm time.Time
	TLatSugerido  time.Duration
	ComAncora     bool
	AckSeq        uint32
	ValidadeS     int
}

// ResponderDesafio valida a resposta, ancora o tempo e emite o token
// (POST /v1/dongles/{id}/desafio/resposta).
type ResponderDesafio struct {
	repos repositorio.Repositorios
	tx    repositorio.Transacionador
	kms   porta.Autenticador
	token porta.EmissorToken
	met   porta.MetricasHandshake
	agora porta.Relogio
	log   *slog.Logger
	p     Parametros
}

// NovoResponderDesafio liga o caso de uso às suas portas.
func NovoResponderDesafio(repos repositorio.Repositorios, tx repositorio.Transacionador, kms porta.Autenticador,
	token porta.EmissorToken, met porta.MetricasHandshake, agora porta.Relogio, log *slog.Logger, p Parametros) *ResponderDesafio {
	return &ResponderDesafio{repos: repos, tx: tx, kms: kms, token: token, met: met, agora: agora, log: log, p: p}
}

var errRollback = errors.New("rollback")

func estadoDe(e dongle.Estado) dhs.Estado {
	return dhs.Estado{SeqContiguo: uint32(e.SeqContiguo), HeadSeq: uint32(e.HeadSeq), CConf: uint32(e.CConf),
		GeracaoBoot: uint16(e.GeracaoBoot), UltimoBootID: uint16(e.UltimoBootID)}
}

// Executar processa o TLV 0x02 já extraído do JSON.
func (c *ResponderDesafio) Executar(ctx context.Context, e ResponderDesafioEntrada) (ResultadoHandshake, error) {
	d, err := c.repos.Dongles.BuscarPorDevID(ctx, e.DevID)
	if err != nil {
		return ResultadoHandshake{}, err
	}
	if err := d.VerificarAcesso(c.p.PermitirPerfilNone); err != nil {
		return ResultadoHandshake{}, err
	}
	s2 := dhs.TetoMicro(e.S2)
	t, n, err := protocolo.LerTLV(e.TLV)
	if err != nil || n != len(e.TLV) || t.T != protocolo.TipoResposta {
		return ResultadoHandshake{}, protocolo.ErrTipoInesperado
	}
	r, payload, tag, err := protocolo.DecodificarResposta(t.V)
	if err != nil {
		return ResultadoHandshake{}, err
	}

	// 4. pré-checagem sem lock
	ds, err := c.repos.Desafios.Buscar(ctx, d.ID, int64(r.CS))
	if err != nil {
		if errors.Is(err, dhs.ErrDesafioInvalido) {
			c.met.Handshake("desafio_invalido")
		}
		return ResultadoHandshake{}, err
	}
	if !protocolo.CompararTag(ds.Nonce, r.NS[:]) || ds.RespondidoEm != nil || !ds.ExpiraEm.After(s2) {
		c.met.Handshake("desafio_invalido")
		return ResultadoHandshake{}, dhs.ErrDesafioInvalido
	}
	est, err := c.repos.Estados.Ler(ctx, d.ID)
	if err != nil {
		return ResultadoHandshake{}, err
	}
	if int64(r.CS) <= est.CConf {
		c.met.Handshake("c_s_antigo")
		return ResultadoHandshake{}, dhs.ErrCSAntigo
	}

	// 5. tag K_auth
	macs, err := c.kms.HMACAuth(observabilidade.ComFaseKMS(ctx, observabilidade.FasePre), d.ChaveAuth,
		[][]byte{protocolo.EntradaTag(protocolo.TipoResposta, d.DevID, payload)})
	if err != nil {
		return ResultadoHandshake{}, err
	}
	if !protocolo.CompararTag(tag16(macs[0]), tag) {
		_ = c.repos.Desafios.MarcarTagInvalida(ctx, d.ID, int64(r.CS), r.NS[:])
		c.log.Warn("handshake.tag_invalida", "dongle", d.DevID.Hex(), "c_s", r.CS)
		c.met.Handshake("tag_invalida")
		return ResultadoHandshake{}, dhs.ErrTagInvalida
	}

	// 6. JWT (fora da transação)
	emit, err := c.token.Emitir(ctx, d.DevID, r.CS)
	if err != nil {
		return ResultadoHandshake{}, err
	}

	// 7. transação
	var res ResultadoHandshake
	var erroAposCommit error
	err = c.tx.EmTransacao(ctx, func(ctx context.Context, rp repositorio.Repositorios) error {
		erroAposCommit = nil
		if err := rp.Dongles.Travar(ctx, d.ID); err != nil {
			return err
		}
		bootID := int32(r.BootID)
		head, tail := int64(r.HeadSeq), int64(r.TailSeq)
		cs := int64(r.CS)
		s1, err := rp.Desafios.Consumir(ctx, dhs.Consumo{DongleID: d.ID, CS: cs, Nonce: r.NS[:], S2: s2, BootID: bootID,
			TMs: int64(r.TMs), HeadSeq: head, TailSeq: tail})
		if err != nil {
			if errors.Is(err, dhs.ErrDesafioInvalido) {
				erroAposCommit = dhs.ErrDesafioInvalido
				return errRollback
			}
			return err
		}
		er, err := rp.Estados.Ler(ctx, d.ID)
		if err != nil {
			return err
		}
		est := estadoDe(er)
		if cs <= er.CConf {
			erroAposCommit = dhs.ErrCSAntigo
			return errRollback
		}
		ack, perda, err := dhs.AplicarTail(est, r.TailSeq, r.HeadSeq)
		if err != nil {
			if ferr := rp.Desafios.Finalizar(ctx, dhs.Finalizacao{DongleID: d.ID, CS: cs, Resultado: dhs.ResultadoInconsistente}); ferr != nil {
				return ferr
			}
			c.log.Warn("handshake.inconsistente", "dongle", d.DevID.Hex(), "c_s", r.CS, "head", r.HeadSeq, "tail", r.TailSeq, "head_salvo", est.HeadSeq)
			erroAposCommit = dhs.ErrRespostaInconsist
			return nil
		}
		var perdaIni, perdaFim *int64
		if perda != nil {
			ini, fim := int64(perda.Ini), int64(perda.Fim)
			if err := rp.Ocorrencias.InserirLacuna(ctx, ocorrencia.Lacuna{DongleID: d.ID, From: ini, To: fim,
				Causa: ocorrencia.CausaOverflow, RegistradoEm: s2, CS: &cs}); err != nil {
				return err
			}
			c.met.Lacuna(ocorrencia.CausaOverflow)
			perdaIni, perdaFim = &ini, &fim
		}
		rtt := s2.Sub(s1)
		comAncora, relogio := dhs.AvaliarAncora(s1, s2, c.p.AncoraRTTMax)
		if rtt >= 0 {
			c.met.AncoraRTT(rtt)
		}
		ger := dhs.GeracaoDoHandshake(est, r.BootID)
		gerI := int32(ger)
		if relogio {
			_ = rp.Ocorrencias.InserirAlerta(ctx, ocorrencia.Alerta{DongleID: d.ID, GeracaoBoot: &gerI, BootID: &bootID,
				Tipo: ocorrencia.AlertaRelogioRegrediu, Detalhe: []byte(`{"rtt_ms":` + strconv.FormatInt(rtt.Milliseconds(), 10) + `}`)})
		}
		refs, err := rp.Boots.TMsUltimo(ctx, d.ID, []int32{bootID}, gerI, gerI)
		if err != nil {
			return err
		}
		ref := int64(-1)
		for _, x := range refs {
			if x.Geracao == gerI && x.BootID == bootID {
				ref = x.TMsUltimo
			}
		}
		tMs := tempo.Desembrulhar(r.TMs, ref)
		if err := rp.Boots.GarantirPendente(ctx, d.ID, gerI, bootID, c.agora()); err != nil {
			return err
		}
		if comAncora {
			if err := rp.Boots.InserirAncora(ctx, dhs.Ancora{DongleID: d.ID, GeracaoBoot: gerI, BootID: bootID,
				TMs: tMs, Nonce: r.NS[:], S1: s1, S2: s2}); err != nil {
				return err
			}
		}
		am := protocolo.Ack{AckSeq: ack, CConf: r.CS}
		if !comAncora {
			am.Flags = protocolo.FlagAckAncoraFalhou
		}
		macs, err := c.kms.HMACAuth(observabilidade.ComFaseKMS(ctx, observabilidade.FaseTx), d.ChaveAuth,
			[][]byte{protocolo.EntradaTag(protocolo.TipoAck, d.DevID, am.Payload())})
		if err != nil {
			return err
		}
		ackTLV := am.Montar(tag16(macs[0]))
		ackI := int64(ack)
		if err := rp.Estados.AtualizarHandshake(ctx, dhs.Atualizacao{DongleID: d.ID, SeqContiguo: ackI, HeadSeq: head,
			TailSeq: tail, S2: s2, Agora: c.agora(), CConf: cs, UltimoAck: ackTLV}); err != nil {
			return err
		}
		resultado := dhs.ResultadoAceito
		if !comAncora {
			resultado = dhs.ResultadoAceitoSemAncora
		}
		jti := emit.JTI
		if err := rp.Desafios.Finalizar(ctx, dhs.Finalizacao{DongleID: d.ID, CS: cs, Resultado: resultado, AckSeq: &ackI,
			PerdaIni: perdaIni, PerdaFim: perdaFim, TokenJTI: &jti, TMsDesembrulhado: &tMs, GeracaoBoot: &gerI}); err != nil {
			return err
		}
		if err := rp.Boots.NotificarPendente(ctx, fmt.Sprintf("%d:%d:%d", d.ID, ger, r.BootID)); err != nil {
			return err
		}
		res = ResultadoHandshake{AckTLV: ackTLV, Token: emit.Token, TokenExpiraEm: emit.ExpiraEm, ComAncora: comAncora, AckSeq: ack,
			TLatSugerido: c.p.TLatSugerido, ValidadeS: int(c.token.Validade().Seconds())}
		c.log.Info("handshake.aceito", "dongle", d.DevID.Hex(), "c_s", r.CS, "geracao_boot", ger, "boot_id", r.BootID,
			"rtt_ms", rtt.Milliseconds(), "com_ancora", comAncora, "ack_seq", ack)
		return nil
	})
	if errors.Is(err, errRollback) {
		err = erroAposCommit
	}
	if err != nil {
		return ResultadoHandshake{}, err
	}
	if erroAposCommit != nil {
		c.met.Handshake("inconsistente")
		return ResultadoHandshake{}, erroAposCommit
	}
	if res.ComAncora {
		c.met.Handshake("aceito")
	} else {
		c.met.Handshake("aceito_sem_ancora")
	}
	return res, nil
}

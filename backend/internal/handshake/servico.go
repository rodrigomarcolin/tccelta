package handshake

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/dongles"
	"tccelta/backend/internal/kms"
	"tccelta/backend/internal/metricas"
	"tccelta/backend/internal/protocolo"
	"tccelta/backend/internal/tempo"
	"tccelta/backend/internal/token"
)

// Servico implementa EmitirDesafio e ProcessarResposta.
type Servico struct {
	Pool   *pgxpool.Pool
	KMS    kms.Autenticador
	Tokens *token.Emissor
	Agora  func() time.Time
	Rand   io.Reader
	Cfg    config.ProtocoloConfig
	Log    *slog.Logger
	Met    *metricas.M
}

// DesafioEmitido é o resultado de EmitirDesafio.
type DesafioEmitido struct {
	TLV      []byte
	ExpiraEm time.Time
	// RetryAfter é preenchido com ErrDesafiosPendentes.
	RetryAfter time.Duration
}

// ResultadoHandshake é o resultado de ProcessarResposta.
type ResultadoHandshake struct {
	AckTLV        []byte
	Token         string
	TokenExpiraEm time.Time
	TLatSugerido  time.Duration
	ComAncora     bool
	AckSeq        uint32
	ValidadeS     int
}

func (s *Servico) agora() time.Time {
	if s.Agora != nil {
		return s.Agora()
	}
	return time.Now()
}

func (s *Servico) rand() io.Reader {
	if s.Rand != nil {
		return s.Rand
	}
	return rand.Reader
}

func (s *Servico) cont(r string) {
	if s.Met != nil {
		s.Met.Handshakes.WithLabelValues(r).Inc()
	}
}

func tag16(mac []byte) []byte { return mac[:protocolo.TamTagAuth] }

// EmitirDesafio gera o Desafio 0x01 assinado com K_auth.
func (s *Servico) EmitirDesafio(ctx context.Context, d dongles.Dongle) (DesafioEmitido, error) {
	var ns [16]byte
	if _, err := io.ReadFull(s.rand(), ns[:]); err != nil {
		return DesafioEmitido{}, err
	}
	s1 := truncarMicro(s.agora())
	expira := s1.Add(s.Cfg.DesafioValidade)
	q := consultas.New(s.Pool)
	row, err := q.EmitirDesafio(ctx, consultas.EmitirDesafioParams{
		AgoraS: s1.Unix(), DongleID: d.ID, S1: s1, MaxPendentes: int32(s.Cfg.DesafioMaxPendentes),
		Nonce: ns[:], ExpiraEm: expira, FlagsC: 0,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ra := s.pendenteMaisAntigo(ctx, q, d.ID, s1)
			return DesafioEmitido{RetryAfter: ra}, ErrDesafiosPendentes
		}
		if codigoBanco(err) {
			return DesafioEmitido{}, fmt.Errorf("%w: %v", banco.ErrBancoOcupado, err)
		}
		return DesafioEmitido{}, err
	}
	des := protocolo.Desafio{CS: uint32(row.CS), NS: ns, AckSeq: uint32(row.SeqContiguo), CConf: uint32(row.CConf)}
	macs, err := s.KMS.HMACAuth(kms.ComFase(ctx, kms.FasePre), d,
		[][]byte{protocolo.EntradaTag(protocolo.TipoDesafio, d.DevID, des.Payload())})
	if err != nil {
		return DesafioEmitido{}, err
	}
	tlv, err := des.Montar(tag16(macs[0]))
	if err != nil {
		return DesafioEmitido{}, err
	}
	s.Log.Info("handshake.desafio_emitido", "dongle", d.DevID.Hex(), "c_s", des.CS, "ack_seq", des.AckSeq)
	s.cont("desafio_emitido")
	return DesafioEmitido{TLV: tlv, ExpiraEm: expira}, nil
}

func codigoBanco(err error) bool {
	return errors.Is(err, banco.ErrBancoOcupado)
}

// pendenteMaisAntigo: segundos até o pendente mais antigo vencer (mínimo 1 s). Sem consulta dedicada,
// usa a validade do desafio como teto conservador.
func (s *Servico) pendenteMaisAntigo(ctx context.Context, q *consultas.Queries, id int32, ref time.Time) time.Duration {
	return time.Second * time.Duration(max(1, int(s.Cfg.DesafioValidade.Seconds())/3))
}

func estadoDe(r consultas.LerEstadoRow) Estado {
	return Estado{SeqContiguo: uint32(r.SeqContiguo), HeadSeq: uint32(r.HeadSeq), CConf: uint32(r.CConf),
		GeracaoBoot: uint16(r.GeracaoBoot), UltimoBootID: uint16(r.UltimoBootID)}
}

// ProcessarResposta processa o TLV 0x02 já extraído do JSON. s2 é o instante de chegada.
func (s *Servico) ProcessarResposta(ctx context.Context, d dongles.Dongle, s2 time.Time, tlv []byte) (ResultadoHandshake, error) {
	s2 = tetoMicro(s2)
	t, n, err := protocolo.LerTLV(tlv)
	if err != nil || n != len(tlv) || t.T != protocolo.TipoResposta {
		return ResultadoHandshake{}, protocolo.ErrTipoInesperado
	}
	r, payload, tag, err := protocolo.DecodificarResposta(t.V)
	if err != nil {
		return ResultadoHandshake{}, err
	}
	q := consultas.New(s.Pool)

	// 4. pré-checagem sem lock
	ds, err := q.BuscarDesafio(ctx, d.ID, int64(r.CS))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.cont("desafio_invalido")
			return ResultadoHandshake{}, ErrDesafioInvalido
		}
		return ResultadoHandshake{}, err
	}
	if !protocolo.CompararTag(ds.Nonce, r.NS[:]) || ds.RespondidoEm != nil || !ds.ExpiraEm.After(s2) {
		s.cont("desafio_invalido")
		return ResultadoHandshake{}, ErrDesafioInvalido
	}
	est, err := q.LerEstado(ctx, d.ID)
	if err != nil {
		return ResultadoHandshake{}, err
	}
	if int64(r.CS) <= est.CConf {
		s.cont("c_s_antigo")
		return ResultadoHandshake{}, ErrCSAntigo
	}

	// 5. tag K_auth
	macs, err := s.KMS.HMACAuth(kms.ComFase(ctx, kms.FasePre), d,
		[][]byte{protocolo.EntradaTag(protocolo.TipoResposta, d.DevID, payload)})
	if err != nil {
		return ResultadoHandshake{}, err
	}
	if !protocolo.CompararTag(tag16(macs[0]), tag) {
		_ = q.MarcarTagInvalida(ctx, d.ID, int64(r.CS), r.NS[:])
		s.Log.Warn("handshake.tag_invalida", "dongle", d.DevID.Hex(), "c_s", r.CS)
		s.cont("tag_invalida")
		return ResultadoHandshake{}, ErrTagInvalida
	}

	// 6. JWT (fora da transação)
	emit, err := s.Tokens.Emitir(ctx, d.DevID, r.CS)
	if err != nil {
		return ResultadoHandshake{}, err
	}

	// 7. transação
	var res ResultadoHandshake
	var erroAposCommit error
	err = banco.EmTransacao(ctx, s.Pool, banco.OpcoesPadrao, func(ctx context.Context, q *consultas.Queries, tx pgx.Tx) error {
		erroAposCommit = nil
		if err := q.TravarDongle(ctx, d.ID); err != nil {
			return err
		}
		bootID32 := int32(r.BootID)
		tMsCru := int64(r.TMs)
		head, tail := int64(r.HeadSeq), int64(r.TailSeq)
		s1, err := q.ConsumirDesafio(ctx, consultas.ConsumirDesafioParams{S2: &s2, BootID: &bootID32, TMs: &tMsCru,
			HeadSeq: &head, TailSeq: &tail, DongleID: d.ID, CS: int64(r.CS), Nonce: r.NS[:]})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				erroAposCommit = ErrDesafioInvalido
				return errRollback
			}
			return err
		}
		er, err := q.LerEstado(ctx, d.ID)
		if err != nil {
			return err
		}
		e := estadoDe(er)
		if int64(r.CS) <= er.CConf {
			erroAposCommit = ErrCSAntigo
			return errRollback
		}
		ack, perda, err := aplicarTail(e, r.TailSeq, r.HeadSeq)
		if err != nil {
			res := "inconsistente"
			if ferr := q.FinalizarDesafio(ctx, consultas.FinalizarDesafioParams{Resultado: &res, DongleID: d.ID, CS: int64(r.CS)}); ferr != nil {
				return ferr
			}
			s.Log.Warn("handshake.inconsistente", "dongle", d.DevID.Hex(), "c_s", r.CS, "head", r.HeadSeq, "tail", r.TailSeq, "head_salvo", e.HeadSeq)
			erroAposCommit = ErrRespostaInconsist
			return nil
		}
		var perdaIni, perdaFim *int64
		if perda != nil {
			ini, fim := int64(perda.Ini), int64(perda.Fim)
			cs := int64(r.CS)
			if err := q.InserirLacuna(ctx, consultas.InserirLacunaParams{DongleID: d.ID, FromSeq: ini, ToSeq: fim,
				Causa: "OVERFLOW", RegistradoEm: s2, CS: &cs}); err != nil {
				return err
			}
			if s.Met != nil {
				s.Met.Lacunas.WithLabelValues("OVERFLOW").Inc()
			}
			perdaIni, perdaFim = &ini, &fim
		}
		rtt := s2.Sub(s1)
		comAncora, relogio := avaliarAncora(s1, s2, s.Cfg.AncoraRTTMax)
		if s.Met != nil && rtt >= 0 {
			s.Met.AncoraRTT.Observe(rtt.Seconds())
		}
		ger := geracaoDoHandshake(e, r.BootID)
		gerI := int32(ger)
		if relogio {
			_ = q.InserirAlerta(ctx, consultas.InserirAlertaParams{DongleID: d.ID, GeracaoBoot: &gerI, BootID: &bootID32,
				Tipo: "relogio_servidor_regrediu", Detalhe: []byte(`{"rtt_ms":` + strconv.FormatInt(rtt.Milliseconds(), 10) + `}`)})
		}
		refs, err := q.LerTMsUltimoBoots(ctx, d.ID, []int32{bootID32}, gerI, gerI)
		if err != nil {
			return err
		}
		ref := int64(-1)
		for _, x := range refs {
			if x.GeracaoBoot == gerI && x.BootID == bootID32 && x.TMsUltimo != nil {
				ref = *x.TMsUltimo
			}
		}
		tMs := tempo.Desembrulhar(r.TMs, ref)
		if err := q.GarantirBootPendente(ctx, d.ID, gerI, bootID32, s.agora()); err != nil {
			return err
		}
		if comAncora {
			if err := q.InserirAncora(ctx, consultas.InserirAncoraParams{DongleID: d.ID, GeracaoBoot: gerI, BootID: bootID32,
				TMs: tMs, Nonce: r.NS[:], S1: s1, S2: s2}); err != nil {
				return err
			}
		}
		am := protocolo.Ack{AckSeq: ack, CConf: r.CS}
		if !comAncora {
			am.Flags = protocolo.FlagAckAncoraFalhou
		}
		macs, err := s.KMS.HMACAuth(kms.ComFase(ctx, kms.FaseTx), d,
			[][]byte{protocolo.EntradaTag(protocolo.TipoAck, d.DevID, am.Payload())})
		if err != nil {
			return err
		}
		ackTLV := am.Montar(tag16(macs[0]))
		agora := s.agora()
		ackI := int64(ack)
		if err := q.AtualizarEstadoHandshake(ctx, consultas.AtualizarEstadoHandshakeParams{
			SeqContiguo: ackI, HeadSeq: head, TailSeq: &tail, S2: &s2, CConf: int64(r.CS), UltimoAck: ackTLV,
			Agora: &agora, DongleID: d.ID}); err != nil {
			return err
		}
		resu := "aceito"
		if !comAncora {
			resu = "aceito_sem_ancora"
		}
		jti := emit.JTI
		if err := q.FinalizarDesafio(ctx, consultas.FinalizarDesafioParams{Resultado: &resu, AckSeq: &ackI,
			PerdaIni: perdaIni, PerdaFim: perdaFim, TokenJti: &jti, TMsDesembrulhado: &tMs, GeracaoBoot: &gerI,
			DongleID: d.ID, CS: int64(r.CS)}); err != nil {
			return err
		}
		if err := q.NotificarBootPendente(ctx, fmt.Sprintf("%d:%d:%d", d.ID, ger, r.BootID)); err != nil {
			return err
		}
		res = ResultadoHandshake{AckTLV: ackTLV, Token: emit.Token, TokenExpiraEm: emit.ExpiraEm, ComAncora: comAncora, AckSeq: ack,
			TLatSugerido: s.Cfg.TLatSugerido, ValidadeS: int(s.Tokens.Validade.Seconds())}
		s.Log.Info("handshake.aceito", "dongle", d.DevID.Hex(), "c_s", r.CS, "geracao_boot", ger, "boot_id", r.BootID,
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
		s.cont("inconsistente")
		return ResultadoHandshake{}, erroAposCommit
	}
	if res.ComAncora {
		s.cont("aceito")
	} else {
		s.cont("aceito_sem_ancora")
	}
	return res, nil
}

var errRollback = errors.New("rollback")

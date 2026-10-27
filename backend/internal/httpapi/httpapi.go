// Package httpapi traduz HTTP para os serviços (handshake, ingestão) e vice-versa.
package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/dongles"
	"tccelta/backend/internal/handshake"
	"tccelta/backend/internal/ingestao"
	"tccelta/backend/internal/kms"
	"tccelta/backend/internal/limites"
	"tccelta/backend/internal/metricas"
	"tccelta/backend/internal/protocolo"
	"tccelta/backend/internal/token"
)

// ServicoHandshake é o que o handler precisa do serviço de handshake.
type ServicoHandshake interface {
	EmitirDesafio(ctx context.Context, d dongles.Dongle) (handshake.DesafioEmitido, error)
	ProcessarResposta(ctx context.Context, d dongles.Dongle, s2 time.Time, tlv []byte) (handshake.ResultadoHandshake, error)
}

// ServicoIngestao é o que o handler precisa do serviço de ingestão.
type ServicoIngestao interface {
	ProcessarLote(ctx context.Context, d dongles.Dongle, s token.Sessao, corpo []byte) (ingestao.ResultadoLote, error)
	UltimaConfirmacao(ctx context.Context, d dongles.Dongle) ([]byte, error)
}

// BuscadorDongle resolve DevId -> dongle.
type BuscadorDongle interface {
	Buscar(ctx context.Context, dev protocolo.DevID) (dongles.Dongle, error)
}

// Deps são as dependências do roteador.
type Deps struct {
	Cfg       config.Config
	Log       *slog.Logger
	Met       *metricas.M
	Handshake ServicoHandshake
	Ingestao  ServicoIngestao
	Dongles   BuscadorDongle
	Validador *token.Validador
	Limitador limites.Limitador
	Semaforo  *limites.Concorrencia
	Prontidao func() bool // banco, Vault, PIDs, chave pública
	NaoPronto *atomic.Bool
	TLatSug   time.Duration
}

type chaveCtx int

const (
	ctxS2 chaveCtx = iota
	ctxDongle
	ctxSessao
	ctxReqID
)

// problema escreve application/problem+json (RFC 9457).
func problema(w http.ResponseWriter, status int, codigo, detalhe string, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "https://tccelta.local/erros/" + codigo, "title": codigo, "status": status, "codigo": codigo, "detail": detalhe})
}

func (d *Deps) retry503() time.Duration {
	return time.Duration(d.Cfg.Limites.RetryAfterSobrecarga)*time.Second + time.Duration(rand.Intn(3000))*time.Millisecond
}

// erro mapeia erros de serviço para a resposta HTTP.
func (d *Deps) erro(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dongles.ErrDesconhecido):
		problema(w, 404, "dongle_desconhecido", "DevId sem cadastro", 0)
	case errors.Is(err, handshake.ErrDesafiosPendentes):
		problema(w, 429, "desafios_pendentes", "há desafios pendentes demais", time.Second)
	case errors.Is(err, handshake.ErrDesafioInvalido):
		problema(w, 409, "desafio_invalido", "desafio desconhecido, vencido ou já usado", 0)
	case errors.Is(err, handshake.ErrCSAntigo):
		problema(w, 409, "c_s_antigo", "c_S não é maior que o último confirmado", 0)
	case errors.Is(err, handshake.ErrTagInvalida):
		problema(w, 422, "tag_invalida", "tag da resposta inválida", 0)
	case errors.Is(err, handshake.ErrRespostaInconsist):
		problema(w, 422, "resposta_inconsistente", "TAIL/HEAD inconsistentes com o estado", 0)
	case errors.Is(err, ingestao.ErrLoteGrande):
		problema(w, 429, "lote_grande", "lote com TLVs demais", time.Second)
	case errors.Is(err, ingestao.ErrLoteMalformado):
		problema(w, 400, "lote_malformado", err.Error(), 0)
	case errors.Is(err, ingestao.ErrSemConfirmacao):
		problema(w, 404, "sem_confirmacao", "nunca houve handshake aceito", 0)
	case errors.Is(err, protocolo.ErrTipoInesperado), errors.Is(err, protocolo.ErrTLVCurto), errors.Is(err, protocolo.ErrTLVComprimento),
		errors.Is(err, protocolo.ErrTamanhoInvalido), errors.Is(err, protocolo.ErrFlagsReservadas):
		problema(w, 400, "tlv_invalido", err.Error(), 0)
	case errors.Is(err, kms.ErrIndisponivel):
		d.rej("kms")
		problema(w, 503, "kms_indisponivel", "KMS indisponível", d.retry503())
	case errors.Is(err, banco.ErrBancoOcupado):
		d.rej("banco_ocupado")
		problema(w, 503, "banco_ocupado", "banco ocupado", d.retry503())
	default:
		d.Log.Error("http.erro_interno", "erro", err)
		problema(w, 500, "erro_interno", "erro interno", 0)
	}
}

func (d *Deps) rej(m string) {
	if d.Met != nil {
		d.Met.Rejeicoes.WithLabelValues(m).Inc()
	}
}

type respWriter struct {
	http.ResponseWriter
	status int
}

func (r *respWriter) WriteHeader(s int) { r.status = s; r.ResponseWriter.WriteHeader(s) }

type mw = func(http.Handler) http.Handler

func recuperar(log *slog.Logger) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if x := recover(); x != nil {
					log.Error("http.panic", "erro", fmt.Sprint(x), "pilha", string(debug.Stack()))
					problema(w, 500, "erro_interno", "erro interno", 0)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// instrumentar: X-Request-Id, log de acesso e métricas HTTP.
func (d *Deps) instrumentar(rota string) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t0 := time.Now()
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = uuid.NewString()
			}
			w.Header().Set("X-Request-Id", id)
			rw := &respWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), ctxReqID, id)))
			dur := time.Since(t0)
			if d.Met != nil {
				d.Met.HTTPReq.WithLabelValues(rota, strconv.Itoa(rw.status)).Inc()
				d.Met.HTTPDur.WithLabelValues(rota).Observe(dur.Seconds())
			}
			d.Log.Info("http.acesso", "req_id", id, "rota", rota, "status", rw.status, "duracao_ms", dur.Milliseconds())
		})
	}
}

// anotarS2 guarda o instante de chegada antes de qualquer leitura de corpo.
func anotarS2(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxS2, time.Now())))
	})
}

func (d *Deps) ipDe(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		for _, c := range d.Cfg.HTTP.ProxiesConfiaveis {
			if p, err := netip.ParsePrefix(c); err == nil && p.Contains(ip) {
				if x := strings.TrimSpace(r.Header.Get("X-Real-IP")); x != "" {
					return x
				}
			}
		}
	}
	return host
}

func (d *Deps) limitarIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, ra := d.Limitador.Permitir(r.Context(), "ip", d.ipDe(r), d.Cfg.Limites.IPMin); !ok {
			d.rej("limite_taxa")
			problema(w, 429, "limite_taxa", "limite por IP", ra)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d *Deps) limitarDevID(escopo string, limite int) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			chave := r.PathValue("id")
			if s, ok := r.Context().Value(ctxSessao).(token.Sessao); ok {
				chave = s.Dev.Hex()
			}
			if ok, ra := d.Limitador.Permitir(r.Context(), escopo, strings.ToLower(chave), limite); !ok {
				d.rej("limite_taxa")
				problema(w, 429, "limite_taxa", "limite por dongle", ra)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (d *Deps) limitarConcorrencia(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lib, ok := d.Semaforo.Adquirir()
		if !ok {
			d.rej("sobrecarga")
			problema(w, 503, "sobrecarga", "servidor ocupado", d.retry503())
			return
		}
		defer lib()
		next.ServeHTTP(w, r)
	})
}

func limitarCorpo(max int64) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

// autenticarJWT valida o Bearer, resolve o dongle e põe Sessao e Dongle no contexto.
func (d *Deps) autenticarJWT(exigirSubIgualID bool) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") || len(h) < 8 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="tccelta"`)
				problema(w, 401, "token_ausente", "Authorization: Bearer ausente", 0)
				return
			}
			s, err := d.Validador.Validar(r.Context(), strings.TrimSpace(h[7:]))
			if err != nil {
				if errors.Is(err, token.ErrTokenExpirado) {
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="expirado"`)
					problema(w, 401, "token_expirado", "token expirado", 0)
				} else {
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
					problema(w, 401, "token_invalido", "token inválido", 0)
				}
				return
			}
			dg, err := d.Dongles.Buscar(r.Context(), s.Dev)
			if err != nil {
				if errors.Is(err, dongles.ErrDesconhecido) {
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
					problema(w, 401, "token_invalido", "token inválido", 0)
					return
				}
				d.erro(w, err)
				return
			}
			if !dg.Ativo {
				problema(w, 403, "dongle_inativo", "dongle desativado", 0)
				return
			}
			if exigirSubIgualID && !strings.EqualFold(r.PathValue("id"), s.Dev.Hex()) {
				problema(w, 403, "token_outro_dongle", "o token pertence a outro dongle", 0)
				return
			}
			ctx := context.WithValue(r.Context(), ctxSessao, s)
			ctx = context.WithValue(ctx, ctxDongle, dg)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// donglePorID resolve {id} (DevId hex) para um dongle ativo e permitido.
func (d *Deps) donglePorID(w http.ResponseWriter, r *http.Request) (dongles.Dongle, bool) {
	dev, err := protocolo.DevIDDeHex(r.PathValue("id"))
	if err != nil {
		problema(w, 400, "devid_invalido", "DevId deve ter 16 dígitos hexadecimais", 0)
		return dongles.Dongle{}, false
	}
	dg, err := d.Dongles.Buscar(r.Context(), dev)
	if err != nil {
		d.erro(w, err)
		return dongles.Dongle{}, false
	}
	if !dg.Ativo {
		problema(w, 403, "dongle_inativo", "dongle desativado", 0)
		return dongles.Dongle{}, false
	}
	if dg.Perfil.Nome == "none" && !d.Cfg.PermitirPerfilNone {
		problema(w, 403, "perfil_nao_permitido", "perfil none não permitido", 0)
		return dongles.Dongle{}, false
	}
	return dg, true
}

type saidaDesafio struct {
	Desafio  string    `json:"desafio"`
	ExpiraEm time.Time `json:"expira_em"`
}

type corpoResposta struct {
	Resposta string `json:"resposta"`
}

type saidaHandshake struct {
	Ack            string    `json:"ack"`
	Token          string    `json:"token"`
	TokenExpiraEm  time.Time `json:"token_expira_em"`
	TokenValidadeS int       `json:"token_validade_s"`
	TLatS          *int      `json:"t_lat_s,omitempty"`
}

func (d *Deps) pedirDesafio(w http.ResponseWriter, r *http.Request) {
	dg, ok := d.donglePorID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 0)
	out, err := d.Handshake.EmitirDesafio(r.Context(), dg)
	if err != nil {
		if errors.Is(err, handshake.ErrDesafiosPendentes) {
			problema(w, 429, "desafios_pendentes", "há desafios pendentes demais", out.RetryAfter)
			return
		}
		d.erro(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(saidaDesafio{Desafio: base64.StdEncoding.EncodeToString(out.TLV), ExpiraEm: out.ExpiraEm.UTC()})
}

func (d *Deps) responderDesafio(w http.ResponseWriter, r *http.Request) {
	s2, _ := r.Context().Value(ctxS2).(time.Time)
	if s2.IsZero() {
		s2 = time.Now()
	}
	dg, ok := d.donglePorID(w, r)
	if !ok {
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		problema(w, 415, "tipo_conteudo", "esperado application/json", 0)
		return
	}
	var c corpoResposta
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problema(w, 413, "corpo_grande", "corpo acima de 2 KiB", 0)
			return
		}
		problema(w, 400, "corpo_malformado", "JSON inválido", 0)
		return
	}
	tlv, err := base64.StdEncoding.Strict().DecodeString(c.Resposta)
	if err != nil {
		problema(w, 400, "base64_invalido", "base64 padrão com padding esperado", 0)
		return
	}
	res, err := d.Handshake.ProcessarResposta(r.Context(), dg, s2, tlv)
	if err != nil {
		d.erro(w, err)
		return
	}
	out := saidaHandshake{Ack: base64.StdEncoding.EncodeToString(res.AckTLV), Token: res.Token,
		TokenExpiraEm: res.TokenExpiraEm.UTC(), TokenValidadeS: res.ValidadeS}
	if res.TLatSugerido > 0 {
		t := int(res.TLatSugerido.Seconds())
		out.TLatS = &t
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

func (d *Deps) receberLote(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/octet-stream") {
		problema(w, 415, "tipo_conteudo", "esperado application/octet-stream", 0)
		return
	}
	corpo, err := io.ReadAll(r.Body)
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problema(w, 413, "corpo_grande", "lote acima do limite", 0)
			return
		}
		problema(w, 400, "lote_malformado", "falha ao ler o corpo", 0)
		return
	}
	s := r.Context().Value(ctxSessao).(token.Sessao)
	dg := r.Context().Value(ctxDongle).(dongles.Dongle)
	res, err := d.Ingestao.ProcessarLote(r.Context(), dg, s, corpo)
	if err != nil {
		d.erro(w, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	if res.Interrompido {
		h.Set("X-Lote-Status", "interrompido")
		h.Set("X-Lote-Motivo", string(res.Motivo))
	} else {
		h.Set("X-Lote-Status", "completo")
	}
	h.Set("X-Lote-Acao", string(res.Acao))
	h.Set("X-Lote-Aceitos", strconv.Itoa(res.Aceitos))
	h.Set("X-Lote-Duplicados", strconv.Itoa(res.Duplicados))
	if res.SemProgresso {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	_, _ = w.Write(res.AckTLV)
}

func (d *Deps) obterConfirmacao(w http.ResponseWriter, r *http.Request) {
	dg := r.Context().Value(ctxDongle).(dongles.Dongle)
	ack, err := d.Ingestao.UltimaConfirmacao(r.Context(), dg)
	if err != nil {
		d.erro(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(ack)
}

// Roteador monta as rotas e a cadeia de middlewares.
func (d *Deps) Roteador() http.Handler {
	mux := http.NewServeMux()
	cadeia := func(h http.Handler, ms ...mw) http.Handler {
		for i := len(ms) - 1; i >= 0; i-- {
			h = ms[i](h)
		}
		return h
	}
	maxLote := int64(d.Cfg.Protocolo.LoteMaxBlocos*1059 + 64)
	base := func(rota string, h http.Handler, ms ...mw) http.Handler {
		return cadeia(h, append([]mw{recuperar(d.Log), d.instrumentar(rota)}, ms...)...)
	}
	mux.Handle("POST /v1/dongles/{id}/desafio", base("desafio", http.HandlerFunc(d.pedirDesafio),
		d.limitarIP, d.limitarDevID("desafio", d.Cfg.Limites.DesafioMin)))
	mux.Handle("POST /v1/dongles/{id}/desafio/resposta", base("resposta", http.HandlerFunc(d.responderDesafio),
		anotarS2, d.limitarIP, d.limitarDevID("resposta", d.Cfg.Limites.RespostaMin)))
	mux.Handle("POST /v1/dongles/blocos", base("blocos", http.HandlerFunc(d.receberLote),
		d.limitarIP, d.autenticarJWT(false), d.limitarDevID("blocos", d.Cfg.Limites.BlocosMin), d.limitarConcorrencia, limitarCorpo(maxLote)))
	mux.Handle("GET /v1/dongles/{id}/confirmacao", base("confirmacao", http.HandlerFunc(d.obterConfirmacao),
		d.limitarIP, d.autenticarJWT(true), d.limitarDevID("confirmacao", 60)))
	mux.HandleFunc("GET /saude/viva", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /saude/pronta", func(w http.ResponseWriter, r *http.Request) {
		if (d.NaoPronto != nil && d.NaoPronto.Load()) || (d.Prontidao != nil && !d.Prontidao()) {
			problema(w, 503, "nao_pronto", "dependências indisponíveis", time.Second)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

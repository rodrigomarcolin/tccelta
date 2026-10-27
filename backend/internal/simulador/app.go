package simulador

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"tccelta/backend/internal/dominio/protocolo"
)

// RespostaLote é o que o app virtual entende de POST /blocos.
type RespostaLote struct {
	Status     int
	Ack        protocolo.Ack
	AckTLV     []byte
	Acao       string
	Motivo     string
	Completo   bool
	RetryAfter time.Duration
	Codigo     string
}

// AppVirtual reimplementa o comportamento do app (spec 09) contra o contrato HTTP.
type AppVirtual struct {
	HTTP   *http.Client
	Base   string
	Dongle *DongleVirtual
	Rel    *Relatorio

	Token       string
	TokenExpira time.Time
	K           int
}

// NovoApp cria um app virtual.
func NovoApp(base string, d *DongleVirtual, h *http.Client, rel *Relatorio) *AppVirtual {
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	return &AppVirtual{HTTP: h, Base: base, Dongle: d, Rel: rel, K: 32}
}

type erroHTTP struct {
	Status     int
	Codigo     string
	RetryAfter time.Duration
}

func (e *erroHTTP) Error() string { return fmt.Sprintf("http %d %s", e.Status, e.Codigo) }

func (a *AppVirtual) fazer(ctx context.Context, rota, m, url, ct string, corpo []byte, hdr map[string]string) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, m, a.Base+url, bytes.NewReader(corpo))
	if err != nil {
		return 0, nil, nil, err
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	t0 := time.Now()
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if a.Rel != nil {
		a.Rel.Observar(rota, resp.StatusCode, time.Since(t0))
	}
	return resp.StatusCode, resp.Header, b, nil
}

func problemaDe(st int, h http.Header, b []byte) error {
	var p struct {
		Codigo string `json:"codigo"`
	}
	_ = json.Unmarshal(b, &p)
	ra, _ := strconv.Atoi(h.Get("Retry-After"))
	return &erroHTTP{Status: st, Codigo: p.Codigo, RetryAfter: time.Duration(ra) * time.Second}
}

// PedirDesafio devolve o TLV 0x01.
func (a *AppVirtual) PedirDesafio(ctx context.Context) ([]byte, error) {
	st, h, b, err := a.fazer(ctx, "desafio", "POST", "/v1/dongles/"+a.Dongle.ch.Dev.Hex()+"/desafio", "", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, problemaDe(st, h, b)
	}
	var o struct {
		Desafio string `json:"desafio"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(o.Desafio)
}

// EnviarResposta envia o TLV 0x02 e devolve (ack, token, expira, status).
func (a *AppVirtual) EnviarResposta(ctx context.Context, resp []byte) (ack []byte, err error) {
	corpo, _ := json.Marshal(map[string]string{"resposta": base64.StdEncoding.EncodeToString(resp)})
	st, h, b, err := a.fazer(ctx, "resposta", "POST", "/v1/dongles/"+a.Dongle.ch.Dev.Hex()+"/desafio/resposta", "application/json", corpo, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, problemaDe(st, h, b)
	}
	var o struct {
		Ack       string `json:"ack"`
		Token     string `json:"token"`
		ValidadeS int    `json:"token_validade_s"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, err
	}
	a.Token = o.Token
	a.TokenExpira = time.Now().Add(time.Duration(o.ValidadeS) * time.Second)
	return base64.StdEncoding.DecodeString(o.Ack)
}

// Handshake executa o handshake completo (app <-> dongle <-> servidor). Repete se a âncora falhar.
func (a *AppVirtual) Handshake(ctx context.Context) error {
	for tent := 0; tent < 3; tent++ {
		des, err := a.PedirDesafio(ctx)
		if err != nil {
			return err
		}
		resp, err := a.Dongle.ProcessarDesafio(des)
		if err != nil {
			return err
		}
		ack, err := a.EnviarResposta(ctx, resp)
		if err != nil {
			return err
		}
		falhou, err := a.Dongle.ProcessarAck(ack)
		if err != nil {
			return err
		}
		if !falhou {
			return nil
		}
	}
	return nil
}

// EnviarLote envia os TLVs e devolve a interpretação da resposta.
func (a *AppVirtual) EnviarLote(ctx context.Context, tlvs [][]byte) (RespostaLote, error) {
	corpo := bytes.Join(tlvs, nil)
	return a.EnviarCorpo(ctx, corpo, a.Token)
}

// EnviarCorpo envia bytes arbitrários com um token (testes negativos).
func (a *AppVirtual) EnviarCorpo(ctx context.Context, corpo []byte, token string) (RespostaLote, error) {
	st, h, b, err := a.fazer(ctx, "blocos", "POST", "/v1/dongles/blocos", "application/octet-stream", corpo,
		map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return RespostaLote{}, err
	}
	r := RespostaLote{Status: st, Acao: h.Get("X-Lote-Acao"), Motivo: h.Get("X-Lote-Motivo"), Completo: h.Get("X-Lote-Status") == "completo"}
	if h.Get("Content-Type") == "application/octet-stream" {
		r.AckTLV = b
		if t, _, err := protocolo.LerTLV(b); err == nil && t.T == protocolo.TipoAck {
			r.Ack, _, _ = protocolo.DecodificarAck(t.V)
		}
		return r, nil
	}
	return r, problemaDe(st, h, b)
}

// Drenar envia lotes até o servidor confirmar tudo (ACK == HEAD) ou o contexto acabar.
func (a *AppVirtual) Drenar(ctx context.Context) error {
	if a.Token == "" || time.Until(a.TokenExpira) < 5*time.Minute {
		if err := a.Handshake(ctx); err != nil {
			return err
		}
	}
	semProgresso := 0
	for ctx.Err() == nil {
		if a.Dongle.AckLocal() >= a.Dongle.Head() {
			return nil
		}
		tlvs := a.Dongle.LerParaEnvio(a.K)
		if len(tlvs) == 0 {
			return nil
		}
		antes := a.Dongle.AckLocal()
		r, err := a.EnviarLote(ctx, tlvs)
		var eh *erroHTTP
		if errors.As(err, &eh) {
			switch eh.Status {
			case 401:
				if err := a.Handshake(ctx); err != nil {
					return err
				}
				continue
			case 429, 503:
				a.K = max(1, a.K/2)
				espera := eh.RetryAfter
				if espera <= 0 {
					espera = time.Second
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(min(espera, 3*time.Second)):
				}
				continue
			}
			return err
		}
		if err != nil {
			return err
		}
		if len(r.AckTLV) > 0 {
			if _, err := a.Dongle.ProcessarAck(r.AckTLV); err != nil {
				return err
			}
		}
		if a.Dongle.AckLocal() == antes {
			semProgresso++
			if semProgresso >= 3 {
				return fmt.Errorf("sem progresso (acao=%s motivo=%s)", r.Acao, r.Motivo)
			}
		} else {
			semProgresso = 0
			if a.K < 32 {
				a.K = min(32, a.K*2)
			}
		}
	}
	return ctx.Err()
}

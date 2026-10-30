package simulator

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

	"tccelta/backend/internal/domain/protocol"
)

// BatchResponse é o que o app virtual entende de POST /blocos.
type BatchResponse struct {
	Status     int
	Ack        protocol.Ack
	AckTLV     []byte
	Action     string
	Reason     string
	Full       bool
	RetryAfter time.Duration
	Code       string
}

// AppVirtual reimplementa o comportamento do app (spec 09) contra o contrato HTTP.
type AppVirtual struct {
	HTTP   *http.Client
	Base   string
	Dongle *DongleVirtual
	Report *Report

	Token        string
	TokenExpires time.Time
	K            int
}

// NewApp cria um app virtual.
func NewApp(base string, d *DongleVirtual, h *http.Client, rel *Report) *AppVirtual {
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	return &AppVirtual{HTTP: h, Base: base, Dongle: d, Report: rel, K: 32}
}

type httpErr struct {
	Status     int
	Code       string
	RetryAfter time.Duration
}

func (e *httpErr) Error() string { return fmt.Sprintf("http %d %s", e.Status, e.Code) }

func (a *AppVirtual) doRequest(ctx context.Context, route, m, url, ct string, body []byte, hdr map[string]string) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, m, a.Base+url, bytes.NewReader(body))
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
	if a.Report != nil {
		a.Report.Observe(route, resp.StatusCode, time.Since(t0))
	}
	return resp.StatusCode, resp.Header, b, nil
}

func problemFrom(st int, h http.Header, b []byte) error {
	var p struct {
		Code string `json:"codigo"`
	}
	_ = json.Unmarshal(b, &p)
	ra, _ := strconv.Atoi(h.Get("Retry-After"))
	return &httpErr{Status: st, Code: p.Code, RetryAfter: time.Duration(ra) * time.Second}
}

// RequestChallenge devolve o TLV 0x01.
func (a *AppVirtual) RequestChallenge(ctx context.Context) ([]byte, error) {
	st, h, b, err := a.doRequest(ctx, "challenge", "POST", "/v1/dongles/"+a.Dongle.ch.Dev.Hex()+"/desafio", "", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, problemFrom(st, h, b)
	}
	var o struct {
		Challenge string `json:"desafio"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(o.Challenge)
}

// SendResponse envia o TLV 0x02 e devolve (ack, token, expira, status).
func (a *AppVirtual) SendResponse(ctx context.Context, resp []byte) (ack []byte, err error) {
	body, _ := json.Marshal(map[string]string{"resposta": base64.StdEncoding.EncodeToString(resp)})
	st, h, b, err := a.doRequest(ctx, "response", "POST", "/v1/dongles/"+a.Dongle.ch.Dev.Hex()+"/desafio/resposta", "application/json", body, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, problemFrom(st, h, b)
	}
	var o struct {
		Ack       string `json:"ack"`
		Token     string `json:"token"`
		ValidityS int    `json:"token_validade_s"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, err
	}
	a.Token = o.Token
	a.TokenExpires = time.Now().Add(time.Duration(o.ValidityS) * time.Second)
	return base64.StdEncoding.DecodeString(o.Ack)
}

// Handshake executa o handshake completo (app <-> dongle <-> servidor). Repete se a âncora falhar.
func (a *AppVirtual) Handshake(ctx context.Context) error {
	for attempt := 0; attempt < 3; attempt++ {
		ch, err := a.RequestChallenge(ctx)
		if err != nil {
			return err
		}
		resp, err := a.Dongle.ProcessChallenge(ch)
		if err != nil {
			return err
		}
		ack, err := a.SendResponse(ctx, resp)
		if err != nil {
			return err
		}
		failed, err := a.Dongle.ProcessAck(ack)
		if err != nil {
			return err
		}
		if !failed {
			return nil
		}
	}
	return nil
}

// SendBatch envia os TLVs e devolve a interpretação da resposta.
func (a *AppVirtual) SendBatch(ctx context.Context, tlvs [][]byte) (BatchResponse, error) {
	body := bytes.Join(tlvs, nil)
	return a.SendBody(ctx, body, a.Token)
}

// SendBody envia bytes arbitrários com um token (testes negativos).
func (a *AppVirtual) SendBody(ctx context.Context, body []byte, token string) (BatchResponse, error) {
	st, h, b, err := a.doRequest(ctx, "blocks", "POST", "/v1/dongles/blocos", "application/octet-stream", body,
		map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return BatchResponse{}, err
	}
	r := BatchResponse{Status: st, Action: h.Get("X-Lote-Acao"), Reason: h.Get("X-Lote-Motivo"), Full: h.Get("X-Lote-Status") == "completo"}
	if h.Get("Content-Type") == "application/octet-stream" {
		r.AckTLV = b
		if t, _, err := protocol.ReadTLV(b); err == nil && t.T == protocol.AckType {
			r.Ack, _, _ = protocol.DecodeAck(t.V)
		}
		return r, nil
	}
	return r, problemFrom(st, h, b)
}

// Drain envia lotes até o servidor confirmar tudo (ACK == HEAD) ou o contexto acabar.
func (a *AppVirtual) Drain(ctx context.Context) error {
	if a.Token == "" || time.Until(a.TokenExpires) < 5*time.Minute {
		if err := a.Handshake(ctx); err != nil {
			return err
		}
	}
	withoutProgress := 0
	for ctx.Err() == nil {
		if a.Dongle.AckLocal() >= a.Dongle.Head() {
			return nil
		}
		tlvs := a.Dongle.ReadForSending(a.K)
		if len(tlvs) == 0 {
			return nil
		}
		before := a.Dongle.AckLocal()
		r, err := a.SendBatch(ctx, tlvs)
		var herr *httpErr
		if errors.As(err, &herr) {
			switch herr.Status {
			case 401:
				if err := a.Handshake(ctx); err != nil {
					return err
				}
				continue
			case 429, 503:
				a.K = max(1, a.K/2)
				wait := herr.RetryAfter
				if wait <= 0 {
					wait = time.Second
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(min(wait, 3*time.Second)):
				}
				continue
			}
			return err
		}
		if err != nil {
			return err
		}
		if len(r.AckTLV) > 0 {
			if _, err := a.Dongle.ProcessAck(r.AckTLV); err != nil {
				return err
			}
		}
		if a.Dongle.AckLocal() == before {
			withoutProgress++
			if withoutProgress >= 3 {
				return fmt.Errorf("sem progresso (acao=%s motivo=%s)", r.Action, r.Reason)
			}
		} else {
			withoutProgress = 0
			if a.K < 32 {
				a.K = min(32, a.K*2)
			}
		}
	}
	return ctx.Err()
}

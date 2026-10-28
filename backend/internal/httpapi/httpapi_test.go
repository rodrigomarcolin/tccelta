package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/domain/dongle"
	dhs "tccelta/backend/internal/domain/handshake"
	ding "tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/limits"
	"tccelta/backend/internal/token"
	hsUC "tccelta/backend/internal/usecase/handshake"
	ingUC "tccelta/backend/internal/usecase/ingestion"
	"tccelta/backend/internal/usecase/port"
	"tccelta/backend/internal/usecase/port/porttest"
	"tccelta/backend/internal/usecase/session"
)

type fakeChallenge struct{ err error }

func (f *fakeChallenge) Run(context.Context, hsUC.IssueChallengeInput) (hsUC.IssuedChallenge, error) {
	return hsUC.IssuedChallenge{TLV: []byte{1, 2, 3}, ExpiresAt: time.Unix(10, 0), RetryAfter: 5 * time.Second}, f.err
}

type fakeResponse struct {
	err error
	s2  time.Time
}

func (f *fakeResponse) Run(_ context.Context, e hsUC.RespondChallengeInput) (hsUC.HandshakeResult, error) {
	f.s2 = e.S2
	return hsUC.HandshakeResult{AckTLV: []byte{4}, Token: "tk", TokenExpiresAt: time.Unix(20, 0), ValidityS: 3600, SuggestedTLat: time.Minute}, f.err
}

type fakeBatch struct {
	res ingUC.BatchResult
	err error
}

func (f *fakeBatch) Run(context.Context, ingUC.ReceiveBatchInput) (ingUC.BatchResult, error) {
	return f.res, f.err
}

type fakeConfirmation struct{}

func (fakeConfirmation) Run(context.Context, ingUC.GetConfirmationInput) ([]byte, error) {
	return nil, ding.ErrNoConfirmation
}

type fakes struct {
	challenge *fakeChallenge
	response  *fakeResponse
	batch     *fakeBatch
}

func newFakes() *fakes {
	return &fakes{challenge: &fakeChallenge{}, response: &fakeResponse{}, batch: &fakeBatch{}}
}

// build liga o roteador a fakes dos casos de uso; só a autenticação é real (validador de token sobre
// o KMS em memória e repositório de dongles em memória).
func build(t *testing.T, f *fakes, active bool) (*Deps, *token.Issuer) {
	t.Helper()
	k := porttest.New()
	v, err := token.NewValidator(context.Background(), k, "iss", "aud", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dev, _ := protocol.DevIDFromHex("0011223344556677")
	cfg, _ := config.LoadFrom(func(string) string { return "" })
	d := &Deps{Cfg: cfg, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Met: porttest.Nop{},
		RequestChallenge: f.challenge, RespondChallenge: f.response, ReceiveBatch: f.batch, GetConfirmation: fakeConfirmation{},
		Authenticate: fakeAuthenticate{v: v, dev: dev, active: active},
		Limiter:      limits.NewLocal(), Semaphore: limits.NewConcurrency(1, 0)}
	return d, token.NewIssuer(k, "iss", "aud", time.Hour)
}

func req(h http.Handler, m, p string, body []byte, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(m, p, bytes.NewReader(body))
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func code(w *httptest.ResponseRecorder) string {
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	c, _ := m["codigo"].(string)
	return c
}

func TestInvalidAndUnknownDevID(t *testing.T) {
	// o caso de uso de desafio real, sobre repositórios em memória, responde 404 ao DevId sem cadastro
	f := newFakes()
	f.challenge.err = dongle.ErrUnknown
	d, _ := build(t, f, true)
	h := d.Router()
	if w := req(h, "POST", "/v1/dongles/zz/desafio", nil, nil); w.Code != 400 || code(w) != "devid_invalido" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := req(h, "POST", "/v1/dongles/0000000000000000/desafio", nil, nil); w.Code != 404 || code(w) != "dongle_desconhecido" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestErrorMapping(t *testing.T) {
	usecases := []struct {
		err    error
		status int
		code   string
		ra     bool
	}{
		{dhs.ErrInvalidChallenge, 409, "desafio_invalido", false},
		{dhs.ErrStaleCS, 409, "c_s_antigo", false},
		{dhs.ErrInvalidTag, 422, "tag_invalida", false},
		{dhs.ErrInconsistentResponse, 422, "resposta_inconsistente", false},
		{dongle.ErrInactive, 403, "dongle_inativo", false},
		{dongle.ErrProfileNotAllowed, 403, "perfil_nao_permitido", false},
		{port.ErrKMSUnavailable, 503, "kms_indisponivel", true},
		{port.ErrDatabaseBusy, 503, "banco_ocupado", true},
		{protocol.ErrUnexpectedType, 400, "tlv_invalido", false},
	}
	for _, c := range usecases {
		f := newFakes()
		f.response.err = c.err
		d, _ := build(t, f, true)
		body, _ := json.Marshal(responseBody{Response: base64.StdEncoding.EncodeToString([]byte{2})})
		w := req(d.Router(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", body, map[string]string{"Content-Type": "application/json"})
		if w.Code != c.status || code(w) != c.code || (w.Header().Get("Retry-After") != "") != c.ra {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body.String())
		}
	}
}

func TestPendingChallengesWithRetryAfter(t *testing.T) {
	f := newFakes()
	f.challenge.err = dhs.ErrPendingChallenges
	d, _ := build(t, f, true)
	w := req(d.Router(), "POST", "/v1/dongles/0011223344556677/desafio", nil, nil)
	if w.Code != 429 || code(w) != "desafios_pendentes" || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body.String(), w.Header())
	}
}

func TestResponseOK(t *testing.T) {
	f := newFakes()
	d, _ := build(t, f, true)
	before := time.Now()
	body, _ := json.Marshal(responseBody{Response: base64.StdEncoding.EncodeToString([]byte{2})})
	w := req(d.Router(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", body, map[string]string{"Content-Type": "application/json"})
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	var s handshakeOutput
	_ = json.Unmarshal(w.Body.Bytes(), &s)
	if s.Token != "tk" || s.TokenValidityS != 3600 || s.TLatS == nil || *s.TLatS != 60 {
		t.Fatalf("%+v", s)
	}
	if f.response.s2.Before(before) {
		t.Fatal("S2 deve ser anotado na chegada")
	}
	// base64 inválido (sem padding)
	body2, _ := json.Marshal(responseBody{Response: "Ag"})
	if w := req(d.Router(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", body2, map[string]string{"Content-Type": "application/json"}); w.Code != 400 || code(w) != "base64_invalido" {
		t.Fatal(w.Code, w.Body.String())
	}
	// Content-Type errado
	if w := req(d.Router(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", body, map[string]string{"Content-Type": "text/plain"}); w.Code != 415 {
		t.Fatal(w.Code)
	}
}

func TestAuthenticationBlocks(t *testing.T) {
	f := newFakes()
	f.batch.res = ingUC.BatchResult{AckTLV: []byte{4, 25, 0}, Action: ding.ActionReread, Reason: ding.ReasonInvalidTag,
		Interrupted: true, NoProgress: true}
	d, em := build(t, f, true)
	h := d.Router()
	oct := map[string]string{"Content-Type": "application/octet-stream"}
	if w := req(h, "POST", "/v1/dongles/blocos", []byte{1}, oct); w.Code != 401 || code(w) != "token_ausente" || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatal(w.Code)
	}
	if w := req(h, "POST", "/v1/dongles/blocos", []byte{1}, map[string]string{"Authorization": "Bearer lixo", "Content-Type": "application/octet-stream"}); w.Code != 401 || code(w) != "token_invalido" {
		t.Fatal(w.Code)
	}
	dev, _ := protocol.DevIDFromHex("0011223344556677")
	tk, _ := em.Issue(context.Background(), dev, 7)
	oct["Authorization"] = "Bearer " + tk.Token
	w := req(h, "POST", "/v1/dongles/blocos", []byte{1}, oct)
	if w.Code != 422 || w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("X-Lote-Acao") != "reler" ||
		w.Header().Get("X-Lote-Motivo") != "tag_invalida" || w.Header().Get("X-Lote-Status") != "interrompido" {
		t.Fatal(w.Code, w.Header())
	}
	// content-type errado
	oct2 := map[string]string{"Authorization": "Bearer " + tk.Token, "Content-Type": "text/plain"}
	if w := req(h, "POST", "/v1/dongles/blocos", []byte{1}, oct2); w.Code != 415 {
		t.Fatal(w.Code)
	}
	// confirmação com sub diferente
	if w := req(h, "GET", "/v1/dongles/0011223344556678/confirmacao", nil, map[string]string{"Authorization": "Bearer " + tk.Token}); w.Code != 403 || code(w) != "token_outro_dongle" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := req(h, "GET", "/v1/dongles/0011223344556677/confirmacao", nil, map[string]string{"Authorization": "Bearer " + tk.Token}); w.Code != 404 || code(w) != "sem_confirmacao" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestInactiveDongleAndSemaphore(t *testing.T) {
	d, em := build(t, newFakes(), false)
	dev, _ := protocol.DevIDFromHex("0011223344556677")
	tk, _ := em.Issue(context.Background(), dev, 7)
	hdr := map[string]string{"Authorization": "Bearer " + tk.Token, "Content-Type": "application/octet-stream"}
	if w := req(d.Router(), "POST", "/v1/dongles/blocos", []byte{1}, hdr); w.Code != 403 || code(w) != "dongle_inativo" {
		t.Fatal(w.Code)
	}
	d2, em2 := build(t, newFakes(), true)
	tk2, _ := em2.Issue(context.Background(), dev, 7)
	hdr["Authorization"] = "Bearer " + tk2.Token
	lib, _ := d2.Semaphore.Acquire() // ocupa a única vaga
	defer lib()
	w := req(d2.Router(), "POST", "/v1/dongles/blocos", []byte{1}, hdr)
	if w.Code != 503 || code(w) != "sobrecarga" || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestHealth(t *testing.T) {
	d, _ := build(t, newFakes(), true)
	ready := true
	d.Readiness = func() bool { return ready }
	h := d.Router()
	if req(h, "GET", "/saude/viva", nil, nil).Code != 200 || req(h, "GET", "/saude/pronta", nil, nil).Code != 200 {
		t.Fatal("saude")
	}
	ready = false
	if req(h, "GET", "/saude/pronta", nil, nil).Code != 503 {
		t.Fatal("deveria estar não pronto")
	}
}

// fakeAuthenticate valida o token de verdade (validador sobre o KMS em memória) e simula o cadastro:
// só o dongle `dev` existe, ativo ou não.
type fakeAuthenticate struct {
	v      *token.Validator
	dev    protocol.DevID
	active bool
}

func (f fakeAuthenticate) Run(ctx context.Context, raw string) (session.Authenticated, error) {
	s, err := f.v.Validate(ctx, raw)
	if err != nil {
		return session.Authenticated{}, err
	}
	if s.Dev != f.dev {
		return session.Authenticated{}, port.ErrInvalidToken
	}
	if !f.active {
		return session.Authenticated{}, dongle.ErrInactive
	}
	return session.Authenticated{Session: s, Dongle: dongle.Dongle{DevID: f.dev, Active: true}}, nil
}

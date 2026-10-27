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

	casohs "tccelta/backend/internal/casodeuso/handshake"
	casoing "tccelta/backend/internal/casodeuso/ingestao"
	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/casodeuso/porta/portateste"
	"tccelta/backend/internal/casodeuso/sessao"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/dominio/dongle"
	dhs "tccelta/backend/internal/dominio/handshake"
	ding "tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/limites"
	"tccelta/backend/internal/repositorio/memoria"
	"tccelta/backend/internal/token"
)

type fakeDesafio struct{ err error }

func (f *fakeDesafio) Executar(context.Context, casohs.EmitirDesafioEntrada) (casohs.DesafioEmitido, error) {
	return casohs.DesafioEmitido{TLV: []byte{1, 2, 3}, ExpiraEm: time.Unix(10, 0), RetryAfter: 5 * time.Second}, f.err
}

type fakeResposta struct {
	err error
	s2  time.Time
}

func (f *fakeResposta) Executar(_ context.Context, e casohs.ResponderDesafioEntrada) (casohs.ResultadoHandshake, error) {
	f.s2 = e.S2
	return casohs.ResultadoHandshake{AckTLV: []byte{4}, Token: "tk", TokenExpiraEm: time.Unix(20, 0), ValidadeS: 3600, TLatSugerido: time.Minute}, f.err
}

type fakeLote struct {
	res casoing.ResultadoLote
	err error
}

func (f *fakeLote) Executar(context.Context, casoing.ReceberLoteEntrada) (casoing.ResultadoLote, error) {
	return f.res, f.err
}

type fakeConfirmacao struct{}

func (fakeConfirmacao) Executar(context.Context, casoing.ObterConfirmacaoEntrada) ([]byte, error) {
	return nil, ding.ErrSemConfirmacao
}

type fakes struct {
	desafio  *fakeDesafio
	resposta *fakeResposta
	lote     *fakeLote
}

func novosFakes() *fakes {
	return &fakes{desafio: &fakeDesafio{}, resposta: &fakeResposta{}, lote: &fakeLote{}}
}

// montar liga o roteador a fakes dos casos de uso; só a autenticação é real (validador de token sobre
// o KMS em memória e repositório de dongles em memória).
func montar(t *testing.T, f *fakes, ativo bool) (*Deps, *token.Emissor) {
	t.Helper()
	k := portateste.Nova()
	v, err := token.NovoValidador(context.Background(), k, "iss", "aud", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bd := memoria.Novo()
	dev, _ := protocolo.DevIDDeHex("0011223344556677")
	bd.AdicionarDongle(dongle.Dongle{ID: 1, DevID: dev, Perfil: protocolo.PerfilHMAC, Ativo: ativo})
	cfg, _ := config.CarregarDe(func(string) string { return "" })
	d := &Deps{Cfg: cfg, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Met: portateste.Nop{},
		PedirDesafio: f.desafio, ResponderDesafio: f.resposta, ReceberLote: f.lote, ObterConfirmacao: fakeConfirmacao{},
		Autenticar: sessao.NovoAutenticar(v, bd.Repositorios().Dongles, false),
		Limitador:  limites.NovoLocal(), Semaforo: limites.NovaConcorrencia(1, 0)}
	return d, token.NovoEmissor(k, "iss", "aud", time.Hour)
}

func req(h http.Handler, m, p string, corpo []byte, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(m, p, bytes.NewReader(corpo))
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func codigo(w *httptest.ResponseRecorder) string {
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	c, _ := m["codigo"].(string)
	return c
}

func TestDevIDInvalidoEDesconhecido(t *testing.T) {
	// o caso de uso de desafio real, sobre repositórios em memória, responde 404 ao DevId sem cadastro
	bd := memoria.Novo()
	f := novosFakes()
	d, _ := montar(t, f, true)
	d.PedirDesafio = casohs.NovoEmitirDesafio(bd.Repositorios(), portateste.Nova(), portateste.Nop{}, portateste.RelogioFixo(time.Unix(1, 0)),
		slog.New(slog.NewTextHandler(io.Discard, nil)), casohs.Parametros{})
	h := d.Roteador()
	if w := req(h, "POST", "/v1/dongles/zz/desafio", nil, nil); w.Code != 400 || codigo(w) != "devid_invalido" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := req(h, "POST", "/v1/dongles/0000000000000000/desafio", nil, nil); w.Code != 404 || codigo(w) != "dongle_desconhecido" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestMapeamentoDeErros(t *testing.T) {
	casos := []struct {
		err    error
		status int
		cod    string
		ra     bool
	}{
		{dhs.ErrDesafioInvalido, 409, "desafio_invalido", false},
		{dhs.ErrCSAntigo, 409, "c_s_antigo", false},
		{dhs.ErrTagInvalida, 422, "tag_invalida", false},
		{dhs.ErrRespostaInconsist, 422, "resposta_inconsistente", false},
		{dongle.ErrInativo, 403, "dongle_inativo", false},
		{dongle.ErrPerfilNaoPermitido, 403, "perfil_nao_permitido", false},
		{porta.ErrKMSIndisponivel, 503, "kms_indisponivel", true},
		{porta.ErrBancoOcupado, 503, "banco_ocupado", true},
		{protocolo.ErrTipoInesperado, 400, "tlv_invalido", false},
	}
	for _, c := range casos {
		f := novosFakes()
		f.resposta.err = c.err
		d, _ := montar(t, f, true)
		corpo, _ := json.Marshal(corpoResposta{Resposta: base64.StdEncoding.EncodeToString([]byte{2})})
		w := req(d.Roteador(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", corpo, map[string]string{"Content-Type": "application/json"})
		if w.Code != c.status || codigo(w) != c.cod || (w.Header().Get("Retry-After") != "") != c.ra {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body.String())
		}
	}
}

func TestDesafiosPendentesComRetryAfter(t *testing.T) {
	f := novosFakes()
	f.desafio.err = dhs.ErrDesafiosPendentes
	d, _ := montar(t, f, true)
	w := req(d.Roteador(), "POST", "/v1/dongles/0011223344556677/desafio", nil, nil)
	if w.Code != 429 || codigo(w) != "desafios_pendentes" || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body.String(), w.Header())
	}
}

func TestRespostaOK(t *testing.T) {
	f := novosFakes()
	d, _ := montar(t, f, true)
	antes := time.Now()
	corpo, _ := json.Marshal(corpoResposta{Resposta: base64.StdEncoding.EncodeToString([]byte{2})})
	w := req(d.Roteador(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", corpo, map[string]string{"Content-Type": "application/json"})
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	var s saidaHandshake
	_ = json.Unmarshal(w.Body.Bytes(), &s)
	if s.Token != "tk" || s.TokenValidadeS != 3600 || s.TLatS == nil || *s.TLatS != 60 {
		t.Fatalf("%+v", s)
	}
	if f.resposta.s2.Before(antes) {
		t.Fatal("S2 deve ser anotado na chegada")
	}
	// base64 inválido (sem padding)
	corpo2, _ := json.Marshal(corpoResposta{Resposta: "Ag"})
	if w := req(d.Roteador(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", corpo2, map[string]string{"Content-Type": "application/json"}); w.Code != 400 || codigo(w) != "base64_invalido" {
		t.Fatal(w.Code, w.Body.String())
	}
	// Content-Type errado
	if w := req(d.Roteador(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", corpo, map[string]string{"Content-Type": "text/plain"}); w.Code != 415 {
		t.Fatal(w.Code)
	}
}

func TestBlocosAutenticacao(t *testing.T) {
	f := novosFakes()
	f.lote.res = casoing.ResultadoLote{AckTLV: []byte{4, 25, 0}, Acao: ding.AcaoReler, Motivo: ding.MotivoTagInvalida,
		Interrompido: true, SemProgresso: true}
	d, em := montar(t, f, true)
	h := d.Roteador()
	oct := map[string]string{"Content-Type": "application/octet-stream"}
	if w := req(h, "POST", "/v1/dongles/blocos", []byte{1}, oct); w.Code != 401 || codigo(w) != "token_ausente" || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatal(w.Code)
	}
	if w := req(h, "POST", "/v1/dongles/blocos", []byte{1}, map[string]string{"Authorization": "Bearer lixo", "Content-Type": "application/octet-stream"}); w.Code != 401 || codigo(w) != "token_invalido" {
		t.Fatal(w.Code)
	}
	dev, _ := protocolo.DevIDDeHex("0011223344556677")
	tk, _ := em.Emitir(context.Background(), dev, 7)
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
	if w := req(h, "GET", "/v1/dongles/0011223344556678/confirmacao", nil, map[string]string{"Authorization": "Bearer " + tk.Token}); w.Code != 403 || codigo(w) != "token_outro_dongle" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := req(h, "GET", "/v1/dongles/0011223344556677/confirmacao", nil, map[string]string{"Authorization": "Bearer " + tk.Token}); w.Code != 404 || codigo(w) != "sem_confirmacao" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestDongleInativoESemaforo(t *testing.T) {
	d, em := montar(t, novosFakes(), false)
	dev, _ := protocolo.DevIDDeHex("0011223344556677")
	tk, _ := em.Emitir(context.Background(), dev, 7)
	hdr := map[string]string{"Authorization": "Bearer " + tk.Token, "Content-Type": "application/octet-stream"}
	if w := req(d.Roteador(), "POST", "/v1/dongles/blocos", []byte{1}, hdr); w.Code != 403 || codigo(w) != "dongle_inativo" {
		t.Fatal(w.Code)
	}
	d2, em2 := montar(t, novosFakes(), true)
	tk2, _ := em2.Emitir(context.Background(), dev, 7)
	hdr["Authorization"] = "Bearer " + tk2.Token
	lib, _ := d2.Semaforo.Adquirir() // ocupa a única vaga
	defer lib()
	w := req(d2.Roteador(), "POST", "/v1/dongles/blocos", []byte{1}, hdr)
	if w.Code != 503 || codigo(w) != "sobrecarga" || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSaude(t *testing.T) {
	d, _ := montar(t, novosFakes(), true)
	pronto := true
	d.Prontidao = func() bool { return pronto }
	h := d.Roteador()
	if req(h, "GET", "/saude/viva", nil, nil).Code != 200 || req(h, "GET", "/saude/pronta", nil, nil).Code != 200 {
		t.Fatal("saude")
	}
	pronto = false
	if req(h, "GET", "/saude/pronta", nil, nil).Code != 503 {
		t.Fatal("deveria estar não pronto")
	}
}

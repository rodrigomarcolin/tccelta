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
	"tccelta/backend/internal/dongles"
	"tccelta/backend/internal/handshake"
	"tccelta/backend/internal/ingestao"
	"tccelta/backend/internal/kms"
	"tccelta/backend/internal/kms/kmsteste"
	"tccelta/backend/internal/limites"
	"tccelta/backend/internal/protocolo"
	"tccelta/backend/internal/token"
)

type fakeHS struct {
	errD, errR error
	s2         time.Time
}

func (f *fakeHS) EmitirDesafio(context.Context, dongles.Dongle) (handshake.DesafioEmitido, error) {
	return handshake.DesafioEmitido{TLV: []byte{1, 2, 3}, ExpiraEm: time.Unix(10, 0)}, f.errD
}
func (f *fakeHS) ProcessarResposta(_ context.Context, _ dongles.Dongle, s2 time.Time, _ []byte) (handshake.ResultadoHandshake, error) {
	f.s2 = s2
	return handshake.ResultadoHandshake{AckTLV: []byte{4}, Token: "tk", TokenExpiraEm: time.Unix(20, 0), ValidadeS: 3600, TLatSugerido: time.Minute}, f.errR
}

type fakeIng struct {
	res ingestao.ResultadoLote
	err error
}

func (f *fakeIng) ProcessarLote(context.Context, dongles.Dongle, token.Sessao, []byte) (ingestao.ResultadoLote, error) {
	return f.res, f.err
}
func (f *fakeIng) UltimaConfirmacao(context.Context, dongles.Dongle) ([]byte, error) {
	return nil, ingestao.ErrSemConfirmacao
}

type fakeDongles struct{ ativo bool }

func (f fakeDongles) Buscar(_ context.Context, dev protocolo.DevID) (dongles.Dongle, error) {
	if dev.Hex() == "0000000000000000" {
		return dongles.Dongle{}, dongles.ErrDesconhecido
	}
	return dongles.Dongle{ID: 1, DevID: dev, Perfil: protocolo.PerfilHMAC, Ativo: f.ativo}, nil
}

func montar(t *testing.T, hs *fakeHS, ing *fakeIng, ativo bool) (*Deps, *kmsteste.Memoria, *token.Emissor) {
	t.Helper()
	k := kmsteste.Nova()
	v, err := token.NovoValidador(context.Background(), k, "iss", "aud", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.CarregarDe(func(string) string { return "" })
	d := &Deps{Cfg: cfg, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Handshake: hs, Ingestao: ing,
		Dongles: fakeDongles{ativo}, Validador: v, Limitador: limites.NovoLocal(), Semaforo: limites.NovaConcorrencia(1, 0)}
	return d, k, &token.Emissor{Assinador: k, Emissor: "iss", Audiencia: "aud", Validade: time.Hour}
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
	d, _, _ := montar(t, &fakeHS{}, &fakeIng{}, true)
	h := d.Roteador()
	if w := req(h, "POST", "/v1/dongles/zz/desafio", nil, nil); w.Code != 400 || codigo(w) != "devid_invalido" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := req(h, "POST", "/v1/dongles/0000000000000000/desafio", nil, nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestMapeamentoDeErros(t *testing.T) {
	casos := []struct {
		err    error
		status int
		cod    string
		ra     bool
	}{
		{handshake.ErrDesafioInvalido, 409, "desafio_invalido", false},
		{handshake.ErrCSAntigo, 409, "c_s_antigo", false},
		{handshake.ErrTagInvalida, 422, "tag_invalida", false},
		{handshake.ErrRespostaInconsist, 422, "resposta_inconsistente", false},
		{kms.ErrIndisponivel, 503, "kms_indisponivel", true},
		{protocolo.ErrTipoInesperado, 400, "tlv_invalido", false},
	}
	for _, c := range casos {
		hs := &fakeHS{errR: c.err}
		d, _, _ := montar(t, hs, &fakeIng{}, true)
		corpo, _ := json.Marshal(corpoResposta{Resposta: base64.StdEncoding.EncodeToString([]byte{2})})
		w := req(d.Roteador(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", corpo, map[string]string{"Content-Type": "application/json"})
		if w.Code != c.status || codigo(w) != c.cod || (w.Header().Get("Retry-After") != "") != c.ra {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body.String())
		}
	}
}

func TestRespostaOK(t *testing.T) {
	hs := &fakeHS{}
	d, _, _ := montar(t, hs, &fakeIng{}, true)
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
	if hs.s2.Before(antes) {
		t.Fatal("S2 deve ser anotado na chegada")
	}
	// base64 inválido (sem padding)
	corpo2, _ := json.Marshal(corpoResposta{Resposta: "Ag"})
	if w := req(d.Roteador(), "POST", "/v1/dongles/0011223344556677/desafio/resposta", corpo2, map[string]string{"Content-Type": "application/json"}); w.Code != 400 || codigo(w) != "base64_invalido" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestBlocosAutenticacao(t *testing.T) {
	ing := &fakeIng{res: ingestao.ResultadoLote{AckTLV: []byte{4, 25, 0}, Acao: ingestao.AcaoReler, Motivo: ingestao.MotivoTagInvalida,
		Interrompido: true, SemProgresso: true}}
	d, _, em := montar(t, &fakeHS{}, ing, true)
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
	d, _, em := montar(t, &fakeHS{}, &fakeIng{}, false)
	dev, _ := protocolo.DevIDDeHex("0011223344556677")
	tk, _ := em.Emitir(context.Background(), dev, 7)
	hdr := map[string]string{"Authorization": "Bearer " + tk.Token, "Content-Type": "application/octet-stream"}
	if w := req(d.Roteador(), "POST", "/v1/dongles/blocos", []byte{1}, hdr); w.Code != 403 || codigo(w) != "dongle_inativo" {
		t.Fatal(w.Code)
	}
	d2, _, em2 := montar(t, &fakeHS{}, &fakeIng{}, true)
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
	d, _, _ := montar(t, &fakeHS{}, &fakeIng{}, true)
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

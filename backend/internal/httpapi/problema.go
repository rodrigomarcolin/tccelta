package httpapi

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/dominio/dongle"
	"tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/protocolo"
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

func (d *Deps) rej(motivo string) { d.Met.Rejeicao(motivo) }

// erro mapeia erros dos casos de uso para a resposta HTTP.
func (d *Deps) erro(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dongle.ErrDesconhecido):
		problema(w, 404, "dongle_desconhecido", "DevId sem cadastro", 0)
	case errors.Is(err, dongle.ErrInativo):
		problema(w, 403, "dongle_inativo", "dongle desativado", 0)
	case errors.Is(err, dongle.ErrPerfilNaoPermitido):
		problema(w, 403, "perfil_nao_permitido", "perfil none não permitido", 0)
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
	case errors.Is(err, porta.ErrKMSIndisponivel):
		d.rej("kms")
		problema(w, 503, "kms_indisponivel", "KMS indisponível", d.retry503())
	case errors.Is(err, porta.ErrBancoOcupado):
		d.rej("banco_ocupado")
		problema(w, 503, "banco_ocupado", "banco ocupado", d.retry503())
	default:
		d.Log.Error("http.erro_interno", "erro", err)
		problema(w, 500, "erro_interno", "erro interno", 0)
	}
}

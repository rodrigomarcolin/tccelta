package api

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
)

// problem escreve application/problem+json (RFC 9457). O campo codigo e os códigos em português são contrato de fio com o app.
func problem(w http.ResponseWriter, status int, code, detalhe string, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "https://tccelta.local/erros/" + code, "title": code, "status": status, "codigo": code, "detail": detalhe})
}

func (d *Deps) retry503() time.Duration {
	return time.Duration(d.Cfg.Limits.RetryAfterOverload)*time.Second + time.Duration(rand.Intn(3000))*time.Millisecond //nolint:gosec // G404: jitter, nao e segredo
}

func (d *Deps) rej(reason string) { d.Met.Rejection(reason) }

// err mapeia erros dos casos de uso para a resposta HTTP.
func (d *Deps) err(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dongle.ErrUnknown):
		problem(w, 404, "dongle_desconhecido", "DevId sem cadastro", 0)
	case errors.Is(err, dongle.ErrInactive):
		problem(w, 403, "dongle_inativo", "dongle desativado", 0)
	case errors.Is(err, dongle.ErrProfileNotAllowed):
		problem(w, 403, "perfil_nao_permitido", "perfil none não permitido", 0)
	case errors.Is(err, handshake.ErrPendingChallenges):
		problem(w, 429, "desafios_pendentes", "há desafios pendentes demais", time.Second)
	case errors.Is(err, handshake.ErrInvalidChallenge):
		problem(w, 409, "desafio_invalido", "desafio desconhecido, vencido ou já usado", 0)
	case errors.Is(err, handshake.ErrStaleCS):
		problem(w, 409, "c_s_antigo", "c_S não é maior que o último confirmado", 0)
	case errors.Is(err, handshake.ErrInvalidTag):
		problem(w, 422, "tag_invalida", "tag da resposta inválida", 0)
	case errors.Is(err, handshake.ErrInconsistentResponse):
		problem(w, 422, "resposta_inconsistente", "TAIL/HEAD inconsistentes com o estado", 0)
	case errors.Is(err, ingestion.ErrBatchTooLarge):
		problem(w, 429, "lote_grande", "lote com TLVs demais", time.Second)
	case errors.Is(err, ingestion.ErrMalformedBatch):
		problem(w, 400, "lote_malformado", err.Error(), 0)
	case errors.Is(err, ingestion.ErrNoConfirmation):
		problem(w, 404, "sem_confirmacao", "nunca houve handshake aceito", 0)
	case errors.Is(err, protocol.ErrUnexpectedType), errors.Is(err, protocol.ErrTLVShort), errors.Is(err, protocol.ErrTLVLength),
		errors.Is(err, protocol.ErrInvalidSize), errors.Is(err, protocol.ErrReservedFlags):
		problem(w, 400, "tlv_invalido", err.Error(), 0)
	case errors.Is(err, port.ErrKMSUnavailable):
		d.rej("kms")
		problem(w, 503, "kms_indisponivel", "KMS indisponível", d.retry503())
	case errors.Is(err, port.ErrDatabaseBusy):
		d.rej("database_busy")
		problem(w, 503, "banco_ocupado", "banco ocupado", d.retry503())
	default:
		d.Log.Error("http.erro_interno", "erro", err)
		problem(w, 500, "erro_interno", "erro interno", 0)
	}
}

// wireReasons traduz o motivo interno do lote para o valor de X-Lote-Motivo. Os literais em português
// são contrato de fio com o app; os motivos internos (e as colunas do banco) são em inglês.
var wireReasons = map[ingestion.Reason]string{
	ingestion.ReasonNone:            "",
	ingestion.ReasonInvalidTag:      "tag_invalida",
	ingestion.ReasonJump:            "salto",
	ingestion.ReasonInvalidMarker:   "marcador_invalido",
	ingestion.ReasonUnknownPID:      "pid_desconhecido",
	ingestion.ReasonMalformedRecord: "registro_malformado",
}

// wireReason devolve o valor de contrato de X-Lote-Motivo para o motivo interno.
func wireReason(r ingestion.Reason) string { return wireReasons[r] }

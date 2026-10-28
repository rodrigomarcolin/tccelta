// Package vault fala com o Vault (Transit): cliente HTTP com login AppRole, as operações do KMS
// (HMAC de K_auth, verify de K_rec, assinatura do JWT) e o ciclo de vida da chave do JWT.
package vault

import "errors"

// ErrIndisponivel: timeout, 5xx, disjuntor aberto, token inválido (o serviço responde 503).
var ErrIndisponivel = errors.New("kms_indisponivel")

// Package versao guarda a versão do binário (definida por ldflags no build).
package versao

// Versao é preenchida com -ldflags "-X tccelta/backend/internal/versao.Versao=...".
var Versao = "dev"

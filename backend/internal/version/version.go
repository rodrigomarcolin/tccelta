// Package version guarda a versão do binário (definida por ldflags no build).
package version

// Version é preenchida com -ldflags "-X tccelta/backend/internal/version.Version=...".
var Version = "dev"

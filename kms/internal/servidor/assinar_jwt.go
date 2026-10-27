package servidor

import (
	"encoding/base64"
	"net/http"
)

type reqAssinar struct {
	Entrada string `json:"entrada"`
}

type respAssinar struct {
	Assinatura string `json:"assinatura"`
	Versao     int    `json:"versao"`
}

// assinarJWT: POST /v1/jwt/assinar. Assina a signing string do JWT com a chave Ed25519; devolve a
// assinatura (64 B) e a versão da chave que assinou.
func (s *Servidor) assinarJWT(w http.ResponseWriter, r *http.Request) {
	var req reqAssinar
	if !lerJSON(w, r, &req) {
		return
	}
	entrada, err := base64.StdEncoding.Strict().DecodeString(req.Entrada)
	if err != nil || len(entrada) == 0 || len(entrada) > MaxEntrada {
		problema(w, 400, "entradas_invalidas", "entrada: base64 padrão de 1 a 4096 bytes esperado", 0)
		return
	}
	sig, versao, err := s.Vault.Assinar(r.Context(), entrada)
	if err != nil {
		s.erroVault(w, err)
		return
	}
	escrever(w, respAssinar{Assinatura: b64(sig), Versao: versao})
}

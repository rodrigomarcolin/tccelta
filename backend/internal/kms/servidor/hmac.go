package servidor

import "net/http"

type reqHMAC struct {
	Chave    string   `json:"chave"`
	Entradas []string `json:"entradas"`
}

type respHMAC struct {
	MACs []string `json:"macs"`
}

// hmac: POST /v1/hmac. HMAC-SHA256 completo (32 B) de cada entrada com a chave K_auth `chave`.
func (s *Servidor) hmac(w http.ResponseWriter, r *http.Request) {
	var req reqHMAC
	if !lerJSON(w, r, &req) {
		return
	}
	if !nomeChave.MatchString(req.Chave) {
		problema(w, 400, "chave_invalida", "nome de chave inválido", 0)
		return
	}
	entradas, err := decodificarLista("entradas", req.Entradas)
	if err != nil {
		problema(w, 400, "entradas_invalidas", err.Error(), 0)
		return
	}
	macs, err := s.Vault.HMACAuth(r.Context(), req.Chave, entradas)
	if err != nil {
		s.erroVault(w, err)
		return
	}
	out := respHMAC{MACs: make([]string, len(macs))}
	for i, m := range macs {
		out.MACs[i] = b64(m)
	}
	escrever(w, out)
}

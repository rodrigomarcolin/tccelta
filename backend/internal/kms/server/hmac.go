package server

import "net/http"

type reqHMAC struct {
	Key    string   `json:"key"`
	Inputs []string `json:"inputs"`
}

type respHMAC struct {
	MACs []string `json:"macs"`
}

// hmac: POST /v1/hmac. HMAC-SHA256 completo (32 B) de cada entrada com a chave K_auth `chave`.
func (s *Server) hmac(w http.ResponseWriter, r *http.Request) {
	var req reqHMAC
	if !readJSON(w, r, &req) {
		return
	}
	if !keyName.MatchString(req.Key) {
		problem(w, 400, "chave_invalida", "nome de chave inválido", 0)
		return
	}
	inputs, err := decodeList("inputs", req.Inputs)
	if err != nil {
		problem(w, 400, "entradas_invalidas", err.Error(), 0)
		return
	}
	macs, err := s.Vault.HMACAuth(r.Context(), req.Key, inputs)
	if err != nil {
		s.vaultError(w, err)
		return
	}
	out := respHMAC{MACs: make([]string, len(macs))}
	for i, m := range macs {
		out.MACs[i] = b64(m)
	}
	writeJSON(w, out)
}

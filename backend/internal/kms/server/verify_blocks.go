package server

import "net/http"

type reqVerify struct {
	Key    string   `json:"key"`
	Inputs []string `json:"inputs"`
	Tags   []string `json:"tags"`
}

type respVerify struct {
	Valid []bool `json:"valid"`
}

// verifyBlocks: POST /v1/blocks/verify. Verifica as tags com a chave K_rec `chave` numa só
// chamada ao Vault (batch_input). Tag que não confere é `false`, não erro.
func (s *Server) verifyBlocks(w http.ResponseWriter, r *http.Request) {
	var req reqVerify
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
	tags, err := decodeList("tags", req.Tags)
	if err != nil {
		problem(w, 400, "entradas_invalidas", err.Error(), 0)
		return
	}
	if len(tags) != len(inputs) {
		problem(w, 400, "entradas_invalidas", "entradas e tags com tamanhos diferentes", 0)
		return
	}
	valid, err := s.Vault.VerifyBlocks(r.Context(), req.Key, inputs, tags)
	if err != nil {
		s.vaultError(w, err)
		return
	}
	writeJSON(w, respVerify{Valid: valid})
}

package servidor

import "net/http"

type reqVerificar struct {
	Chave    string   `json:"chave"`
	Entradas []string `json:"entradas"`
	Tags     []string `json:"tags"`
}

type respVerificar struct {
	Validos []bool `json:"validos"`
}

// verificarBlocos: POST /v1/blocos/verificar. Verifica as tags com a chave K_rec `chave` numa só
// chamada ao Vault (batch_input). Tag que não confere é `false`, não erro.
func (s *Servidor) verificarBlocos(w http.ResponseWriter, r *http.Request) {
	var req reqVerificar
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
	tags, err := decodificarLista("tags", req.Tags)
	if err != nil {
		problema(w, 400, "entradas_invalidas", err.Error(), 0)
		return
	}
	if len(tags) != len(entradas) {
		problema(w, 400, "entradas_invalidas", "entradas e tags com tamanhos diferentes", 0)
		return
	}
	validos, err := s.Vault.VerificarBlocos(r.Context(), req.Chave, entradas, tags)
	if err != nil {
		s.erroVault(w, err)
		return
	}
	escrever(w, respVerificar{Validos: validos})
}

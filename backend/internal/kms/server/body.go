package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// keyName: o nome da chave entra no caminho do Vault; só aceita o alfabeto de um DevId hexadecimal
// (ou nomes equivalentes), nunca "/" ou "..".
var keyName = regexp.MustCompile("^[A-Za-z0-9_-]{1,64}$")

// readJSON valida Content-Type e decodifica o corpo de forma estrita. Em caso de erro, já responde.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		problem(w, 415, "tipo_conteudo", "esperado application/json", 0)
		return false
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problem(w, 413, "corpo_grande", "corpo acima do limite", 0)
			return false
		}
		problem(w, 400, "corpo_malformado", "JSON inválido", 0)
		return false
	}
	if dec.More() {
		problem(w, 400, "corpo_malformado", "JSON com conteúdo extra", 0)
		return false
	}
	return true
}

// decodeList valida e decodifica uma lista de itens em base64 padrão com padding.
func decodeList(field string, items []string) ([][]byte, error) {
	if len(items) == 0 || len(items) > MaxItems {
		return nil, fmt.Errorf("%s: esperado de 1 a %d itens", field, MaxItems)
	}
	out := make([][]byte, len(items))
	for i, s := range items {
		b, err := base64.StdEncoding.Strict().DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: base64 padrão com padding esperado", field, i)
		}
		if len(b) == 0 || len(b) > MaxInput {
			return nil, fmt.Errorf("%s[%d]: tamanho fora de 1..%d bytes", field, i, MaxInput)
		}
		out[i] = b
	}
	return out, nil
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

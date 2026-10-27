package servidor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// nomeChave: o nome da chave entra no caminho do Vault; só aceita o alfabeto de um DevId hexadecimal
// (ou nomes equivalentes), nunca "/" ou "..".
var nomeChave = regexp.MustCompile("^[A-Za-z0-9_-]{1,64}$")

// lerJSON valida Content-Type e decodifica o corpo de forma estrita. Em caso de erro, já responde.
func lerJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		problema(w, 415, "tipo_conteudo", "esperado application/json", 0)
		return false
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problema(w, 413, "corpo_grande", "corpo acima do limite", 0)
			return false
		}
		problema(w, 400, "corpo_malformado", "JSON inválido", 0)
		return false
	}
	if dec.More() {
		problema(w, 400, "corpo_malformado", "JSON com conteúdo extra", 0)
		return false
	}
	return true
}

// decodificarLista valida e decodifica uma lista de itens em base64 padrão com padding.
func decodificarLista(campo string, itens []string) ([][]byte, error) {
	if len(itens) == 0 || len(itens) > MaxItens {
		return nil, fmt.Errorf("%s: esperado de 1 a %d itens", campo, MaxItens)
	}
	out := make([][]byte, len(itens))
	for i, s := range itens {
		b, err := base64.StdEncoding.Strict().DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: base64 padrão com padding esperado", campo, i)
		}
		if len(b) == 0 || len(b) > MaxEntrada {
			return nil, fmt.Errorf("%s[%d]: tamanho fora de 1..%d bytes", campo, i, MaxEntrada)
		}
		out[i] = b
	}
	return out, nil
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

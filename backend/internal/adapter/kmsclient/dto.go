package kmsclient

// DTOs do contrato HTTP com o serviço KMS (kms/docs/contrato-http.md). Duplicados de propósito: a API
// não importa nada do módulo kms, a única ligação entre os dois é este contrato.

type reqHMAC struct {
	Key    string   `json:"key"`
	Inputs []string `json:"inputs"` // base64 padrão
}

type respHMAC struct {
	MACs []string `json:"macs"` // base64 padrão, HMAC-SHA256 completo (32 B)
}

type reqVerify struct {
	Key    string   `json:"key"`
	Inputs []string `json:"inputs"`
	Tags   []string `json:"tags"`
}

type respVerify struct {
	Valid []bool `json:"valid"`
}

type reqSign struct {
	Input string `json:"input"` // base64 padrão
}

type respSign struct {
	Signature string `json:"signature"` // base64 padrão, 64 B
	Version   int    `json:"version"`
}

// respKeys: versão da chave -> chave pública Ed25519 em base64 padrão.
type respKeys map[string]string

type problem struct {
	Code   string `json:"codigo"`
	Detail string `json:"detail"`
}

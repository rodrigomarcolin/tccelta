package remotekms

// DTOs do contrato HTTP com o serviço KMS (kms/docs/contrato-http.md). Duplicados de propósito: a API
// não importa nada do módulo kms, a única ligação entre os dois é este contrato.

type reqHMAC struct {
	Key    string   `json:"chave"`
	Inputs []string `json:"entradas"` // base64 padrão
}

type respHMAC struct {
	MACs []string `json:"macs"` // base64 padrão, HMAC-SHA256 completo (32 B)
}

type reqVerify struct {
	Key    string   `json:"chave"`
	Inputs []string `json:"entradas"`
	Tags   []string `json:"tags"`
}

type respVerify struct {
	Valid []bool `json:"validos"`
}

type reqSign struct {
	Input string `json:"entrada"` // base64 padrão
}

type respSign struct {
	Signature string `json:"assinatura"` // base64 padrão, 64 B
	Version   int    `json:"versao"`
}

// respKeys: versão da chave -> chave pública Ed25519 em base64 padrão.
type respKeys map[string]string

type problem struct {
	Code   string `json:"codigo"`
	Detail string `json:"detail"`
}

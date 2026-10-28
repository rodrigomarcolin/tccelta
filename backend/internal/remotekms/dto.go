package kmsremoto

// DTOs do contrato HTTP com o serviço KMS (kms/docs/contrato-http.md). Duplicados de propósito: a API
// não importa nada do módulo kms, a única ligação entre os dois é este contrato.

type reqHMAC struct {
	Chave    string   `json:"chave"`
	Entradas []string `json:"entradas"` // base64 padrão
}

type respHMAC struct {
	MACs []string `json:"macs"` // base64 padrão, HMAC-SHA256 completo (32 B)
}

type reqVerificar struct {
	Chave    string   `json:"chave"`
	Entradas []string `json:"entradas"`
	Tags     []string `json:"tags"`
}

type respVerificar struct {
	Validos []bool `json:"validos"`
}

type reqAssinar struct {
	Entrada string `json:"entrada"` // base64 padrão
}

type respAssinar struct {
	Assinatura string `json:"assinatura"` // base64 padrão, 64 B
	Versao     int    `json:"versao"`
}

// respChaves: versão da chave -> chave pública Ed25519 em base64 padrão.
type respChaves map[string]string

type problema struct {
	Codigo string `json:"codigo"`
	Detail string `json:"detail"`
}

// Package config carrega a configuração do serviço KMS a partir de variáveis de ambiente.
// Segredos aceitam a forma <NOME>_ARQUIVO (Docker secrets).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// HTTPConfig é o servidor do KMS. Fora do ambiente dev, TLS com certificado de cliente (mTLS) é obrigatório.
type HTTPConfig struct {
	Endereco         string
	MetricasEndereco string
	SaudeRotacao     string
	TLSCert          string
	TLSKey           string
	TLSClientCA      string // CA que assina o certificado do cliente (a API)
}

// VaultConfig é a conexão com o Vault (Transit).
type VaultConfig struct {
	Addr            string
	CACert          string
	ClientCert      string
	ClientKey       string
	RoleIDArquivo   string
	SecretIDArquivo string
	TokenArquivo    string
	Timeout         time.Duration
	Retentativas    int
	MountAuth       string
	MountRec        string
	MountJWT        string
	ChaveJWT        string
	MaxSimultaneas  int
}

// JWTConfig tem o que a rotação precisa saber do token emitido pela API.
type JWTConfig struct {
	Validade time.Duration
	Folga    time.Duration
}

// RotacaoConfig é o ciclo de vida da chave do JWT.
type RotacaoConfig struct {
	Periodo     time.Duration
	Verificacao time.Duration
}

// Config é a configuração completa.
type Config struct {
	Ambiente   string
	LogNivel   string
	LogFormato string
	HTTP       HTTPConfig
	Vault      VaultConfig
	JWT        JWTConfig
	Rotacao    RotacaoConfig
}

// Dev diz se o ambiente é de desenvolvimento (permite HTTP simples).
func (c Config) Dev() bool { return c.Ambiente == "dev" }

type leitor struct {
	env  func(string) string
	errs []string
}

func (l *leitor) str(nome, padrao string) string {
	if v := l.env(nome); v != "" {
		return v
	}
	return padrao
}

func (l *leitor) dur(nome string, padrao time.Duration) time.Duration {
	v := l.env(nome)
	if v == "" {
		return padrao
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: duração inválida %q", nome, v))
		return padrao
	}
	return d
}

func (l *leitor) inteiro(nome string, padrao int) int {
	v := l.env(nome)
	if v == "" {
		return padrao
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: inteiro inválido %q", nome, v))
		return padrao
	}
	return n
}

// Carregar lê o ambiente do processo.
func Carregar() (Config, error) { return CarregarDe(os.Getenv) }

// CarregarDe lê de uma função arbitrária (testes).
func CarregarDe(env func(string) string) (Config, error) {
	l := &leitor{env: env}
	var c Config
	c.Ambiente = l.str("AMBIENTE", "dev")
	c.LogNivel = l.str("LOG_NIVEL", "info")
	c.LogFormato = l.str("LOG_FORMATO", "json")
	c.HTTP = HTTPConfig{
		Endereco:         l.str("KMS_ENDERECO", ":8300"),
		MetricasEndereco: l.str("KMS_METRICAS_ENDERECO", ":9300"),
		SaudeRotacao:     l.str("KMS_SAUDE_ROTACAO", ":8092"),
		TLSCert:          l.str("KMS_TLS_CERT", ""),
		TLSKey:           l.str("KMS_TLS_KEY", ""),
		TLSClientCA:      l.str("KMS_TLS_CLIENT_CA", ""),
	}
	c.Vault = VaultConfig{
		Addr:            l.str("VAULT_ADDR", ""),
		CACert:          l.str("VAULT_CACERT", ""),
		ClientCert:      l.str("VAULT_CLIENT_CERT", ""),
		ClientKey:       l.str("VAULT_CLIENT_KEY", ""),
		RoleIDArquivo:   l.str("VAULT_APPROLE_ROLE_ID_ARQUIVO", ""),
		SecretIDArquivo: l.str("VAULT_APPROLE_SECRET_ID_ARQUIVO", ""),
		TokenArquivo:    l.str("VAULT_TOKEN_ARQUIVO", ""),
		Timeout:         l.dur("VAULT_TIMEOUT", 2*time.Second),
		Retentativas:    l.inteiro("VAULT_RETENTATIVAS", 1),
		MountAuth:       l.str("VAULT_MOUNT_AUTH", "transit-auth"),
		MountRec:        l.str("VAULT_MOUNT_REC", "transit-rec"),
		MountJWT:        l.str("VAULT_MOUNT_JWT", "transit-jwt"),
		ChaveJWT:        l.str("VAULT_CHAVE_JWT", "jwt"),
		MaxSimultaneas:  l.inteiro("VAULT_MAX_SIMULTANEAS", 32),
	}
	c.JWT = JWTConfig{
		Validade: l.dur("JWT_VALIDADE", time.Hour),
		Folga:    l.dur("JWT_FOLGA", 30*time.Second),
	}
	c.Rotacao = RotacaoConfig{
		Periodo:     l.dur("JWT_ROTACAO_PERIODO", 720*time.Hour),
		Verificacao: l.dur("JWT_ROTACAO_VERIFICACAO", time.Hour),
	}
	switch c.Ambiente {
	case "dev", "prodlike", "producao":
	default:
		l.errs = append(l.errs, "AMBIENTE deve ser dev, prodlike ou producao")
	}
	if len(l.errs) > 0 {
		return c, errors.New("configuração inválida: " + strings.Join(l.errs, "; "))
	}
	return c, nil
}

// ValidarServidor confere o que só o modo `servir` exige: Vault configurado e, fora de dev, mTLS.
func (c Config) ValidarServidor() error {
	var e []string
	if c.Vault.Addr == "" {
		e = append(e, "VAULT_ADDR não definida")
	}
	if !c.Dev() && (c.HTTP.TLSCert == "" || c.HTTP.TLSKey == "" || c.HTTP.TLSClientCA == "") {
		e = append(e, "fora de AMBIENTE=dev, KMS_TLS_CERT, KMS_TLS_KEY e KMS_TLS_CLIENT_CA (mTLS) são obrigatórias")
	}
	if len(e) > 0 {
		return errors.New("configuração inválida: " + strings.Join(e, "; "))
	}
	return nil
}

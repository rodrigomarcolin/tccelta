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
	Address        string
	MetricsAddress string
	RotationHealth string
	TLSCert        string
	TLSKey         string
	TLSClientCA    string // CA que assina o certificado do cliente (a API)
	TLSClientCN    string // CN exigido no certificado do cliente (fora de dev)
}

// VaultConfig é a conexão com o Vault (Transit).
type VaultConfig struct {
	Addr          string
	CACert        string
	ClientCert    string
	ClientKey     string
	RoleIDFile    string
	SecretIDFile  string
	TokenFile     string
	Timeout       time.Duration
	Retries       int
	MountAuth     string
	MountRec      string
	MountJWT      string
	JWTKey        string
	MaxConcurrent int
}

// JWTConfig tem o que a rotação precisa saber do token emitido pela API.
type JWTConfig struct {
	Validity time.Duration
	Leeway   time.Duration
}

// RotationConfig é o ciclo de vida da chave do JWT.
type RotationConfig struct {
	Period        time.Duration
	CheckInterval time.Duration
}

// Config é a configuração completa.
type Config struct {
	Environment string
	LogLevel    string
	LogFormat   string
	HTTP        HTTPConfig
	Vault       VaultConfig
	JWT         JWTConfig
	Rotation    RotationConfig
}

// Dev diz se o ambiente é de desenvolvimento (permite HTTP simples).
func (c Config) Dev() bool { return c.Environment == "dev" }

type reader struct {
	env  func(string) string
	errs []string
}

func (l *reader) str(name, def string) string {
	if v := l.env(name); v != "" {
		return v
	}
	return def
}

func (l *reader) dur(name string, def time.Duration) time.Duration {
	v := l.env(name)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: duração inválida %q", name, v))
		return def
	}
	return d
}

func (l *reader) integer(name string, def int) int {
	v := l.env(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: inteiro inválido %q", name, v))
		return def
	}
	return n
}

// Load lê o ambiente do processo.
func Load() (Config, error) { return LoadFrom(os.Getenv) }

// LoadFrom lê de uma função arbitrária (testes).
func LoadFrom(env func(string) string) (Config, error) {
	l := &reader{env: env}
	var c Config
	c.Environment = l.str("AMBIENTE", "dev")
	c.LogLevel = l.str("LOG_NIVEL", "info")
	c.LogFormat = l.str("LOG_FORMATO", "json")
	c.HTTP = HTTPConfig{
		Address:        l.str("KMS_ENDERECO", ":8300"),
		MetricsAddress: l.str("KMS_METRICAS_ENDERECO", ":9300"),
		RotationHealth: l.str("KMS_SAUDE_ROTACAO", ":8092"),
		TLSCert:        l.str("KMS_TLS_CERT", ""),
		TLSKey:         l.str("KMS_TLS_KEY", ""),
		TLSClientCA:    l.str("KMS_TLS_CLIENT_CA", ""),
		TLSClientCN:    l.str("KMS_TLS_CLIENTE_CN", "tccelta-api"),
	}
	c.Vault = VaultConfig{
		Addr:          l.str("VAULT_ADDR", ""),
		CACert:        l.str("VAULT_CACERT", ""),
		ClientCert:    l.str("VAULT_CLIENT_CERT", ""),
		ClientKey:     l.str("VAULT_CLIENT_KEY", ""),
		RoleIDFile:    l.str("VAULT_APPROLE_ROLE_ID_ARQUIVO", ""),
		SecretIDFile:  l.str("VAULT_APPROLE_SECRET_ID_ARQUIVO", ""),
		TokenFile:     l.str("VAULT_TOKEN_ARQUIVO", ""),
		Timeout:       l.dur("VAULT_TIMEOUT", 2*time.Second),
		Retries:       l.integer("VAULT_RETENTATIVAS", 1),
		MountAuth:     l.str("VAULT_MOUNT_AUTH", "transit-auth"),
		MountRec:      l.str("VAULT_MOUNT_REC", "transit-rec"),
		MountJWT:      l.str("VAULT_MOUNT_JWT", "transit-jwt"),
		JWTKey:        l.str("VAULT_CHAVE_JWT", "jwt"),
		MaxConcurrent: l.integer("VAULT_MAX_SIMULTANEAS", 32),
	}
	c.JWT = JWTConfig{
		Validity: l.dur("JWT_VALIDADE", time.Hour),
		Leeway:   l.dur("JWT_FOLGA", 30*time.Second),
	}
	c.Rotation = RotationConfig{
		Period:        l.dur("JWT_ROTACAO_PERIODO", 720*time.Hour),
		CheckInterval: l.dur("JWT_ROTACAO_VERIFICACAO", time.Hour),
	}
	switch c.Environment {
	case "dev", "prodlike", "producao":
	default:
		l.errs = append(l.errs, "AMBIENTE deve ser dev, prodlike ou producao")
	}
	if len(l.errs) > 0 {
		return c, errors.New("configuração inválida: " + strings.Join(l.errs, "; "))
	}
	return c, nil
}

// ValidateServer confere o que só o modo `servir` exige: Vault configurado e, fora de dev, mTLS.
func (c Config) ValidateServer() error {
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

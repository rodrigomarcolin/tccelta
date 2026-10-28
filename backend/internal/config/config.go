// Package config carrega a configuração a partir de variáveis de ambiente.
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

type HTTPConfig struct {
	Address        string
	MetricsAddress string
	WorkerHealth   string
	TrustedProxies []string
}

type DatabaseConfig struct {
	URL      string // DSN sem a senha
	Password string
	PoolMax  int32
	PoolMin  int32
}

// KMSConfig descreve o serviço KMS (módulo kms/), alcançado só por HTTP.
type KMSConfig struct {
	URL            string
	CACert         string
	ClientCert     string
	ClientKey      string
	Timeout        time.Duration
	MaxConcurrency int
	VerifyBatch    int
	VerifyParallel int
}

type JWTConfig struct {
	Issuer   string
	Audience string
	Validity time.Duration
	Leeway   time.Duration
}

type ProtocolConfig struct {
	ChallengeValidity    time.Duration
	MaxPendingChallenges int
	AnchorRTTMax         time.Duration
	MaxBatchBlocks       int
	SuggestedTLat        time.Duration
}

type LimitsConfig struct {
	ChallengeMin, ResponseMin, BlocksMin, IPMin int
	MaxConcurrentIngestions                     int
	IngestionSlotWait                           time.Duration
	RetryAfterOverload                          int
	RedisAddress                                string
	RedisPassword                               string
	RedisTimeout                                time.Duration
}

type WorkerConfig struct {
	Concurrency        int
	PollInterval       time.Duration
	ExpirationInterval time.Duration
	GroupWindow        time.Duration
	MaxDriftPPM        float64
	SpanMin            time.Duration
	MinHalfWidthMs     float64
}

type Config struct {
	Environment      string
	LogLevel         string
	LogFormat        string
	HTTP             HTTPConfig
	Database         DatabaseConfig
	KMS              KMSConfig
	JWT              JWTConfig
	Protocol         ProtocolConfig
	Limits           LimitsConfig
	Worker           WorkerConfig
	AllowNoneProfile bool
}

type envReader struct {
	env  func(string) string
	errs []string
}

func (l *envReader) str(name, defaultValue string) string {
	if v := l.env(name); v != "" {
		return v
	}
	return defaultValue
}

// secret lê NOME ou o conteúdo do arquivo apontado por NOME_ARQUIVO.
func (l *envReader) secret(name string) string {
	if file := l.env(name + "_ARQUIVO"); file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			l.errs = append(l.errs, fmt.Sprintf("%s_ARQUIVO: %v", name, err))
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	return l.env(name)
}

func (l *envReader) dur(name string, defaultValue time.Duration) time.Duration {
	v := l.env(name)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: duração inválida %q", name, v))
		return defaultValue
	}
	return d
}

func (l *envReader) integer(name string, defaultValue int) int {
	v := l.env(name)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: inteiro inválido %q", name, v))
		return defaultValue
	}
	return n
}

func (l *envReader) float(name string, defaultValue float64) float64 {
	v := l.env(name)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: número inválido %q", name, v))
		return defaultValue
	}
	return n
}

func (l *envReader) bool(name string) bool {
	b, _ := strconv.ParseBool(l.env(name))
	return b
}

// Load lê o ambiente do processo.
func Load() (Config, error) { return LoadFrom(os.Getenv) }

// LoadFrom lê de uma função arbitrária (testes).
func LoadFrom(env func(string) string) (Config, error) {
	l := &envReader{env: env}
	var c Config
	c.Environment = l.str("AMBIENTE", "dev")
	c.LogLevel = l.str("LOG_NIVEL", "info")
	c.LogFormat = l.str("LOG_FORMATO", "json")
	c.AllowNoneProfile = l.bool("PERMITIR_PERFIL_NONE")

	c.HTTP = HTTPConfig{
		Address:        l.str("HTTP_ENDERECO", ":8080"),
		MetricsAddress: l.str("METRICAS_ENDERECO", ":9090"),
		WorkerHealth:   l.str("SAUDE_ENDERECO_WORKER", ":8090"),
	}
	if p := l.str("PROXIES_CONFIAVEIS", ""); p != "" {
		for _, x := range strings.Split(p, ",") {
			c.HTTP.TrustedProxies = append(c.HTTP.TrustedProxies, strings.TrimSpace(x))
		}
	}
	c.Database = DatabaseConfig{
		URL:      l.str("BANCO_URL", ""),
		Password: l.secret("BANCO_SENHA"),
		PoolMax:  int32(l.integer("BANCO_POOL_MAX", 40)),
		PoolMin:  int32(l.integer("BANCO_POOL_MIN", 4)),
	}
	c.KMS = KMSConfig{
		URL:            l.str("KMS_URL", ""),
		CACert:         l.str("KMS_CACERT", ""),
		ClientCert:     l.str("KMS_CLIENT_CERT", ""),
		ClientKey:      l.str("KMS_CLIENT_KEY", ""),
		Timeout:        l.dur("KMS_TIMEOUT", 2*time.Second),
		MaxConcurrency: l.integer("KMS_MAX_CONCORRENCIA", 32),
		VerifyBatch:    l.integer("KMS_VERIFY_LOTE", 32),
		VerifyParallel: l.integer("KMS_VERIFY_PARALELO", 1),
	}
	c.JWT = JWTConfig{
		Issuer:   l.str("JWT_EMISSOR", "tccelta-api"),
		Audience: l.str("JWT_AUDIENCIA", "tccelta-ingestao"),
		Validity: l.dur("JWT_VALIDADE", time.Hour),
		Leeway:   l.dur("JWT_FOLGA", 30*time.Second),
	}
	c.Protocol = ProtocolConfig{
		ChallengeValidity:    l.dur("DESAFIO_VALIDADE", 60*time.Second),
		MaxPendingChallenges: l.integer("DESAFIO_MAX_PENDENTES", 3),
		AnchorRTTMax:         l.dur("ANCORA_RTT_MAX", 2*time.Second),
		MaxBatchBlocks:       l.integer("LOTE_MAX_BLOCOS", 32),
		SuggestedTLat:        l.dur("T_LAT_SUGERIDO", 60*time.Second),
	}
	c.Limits = LimitsConfig{
		ChallengeMin:            l.integer("LIMITE_DESAFIO_MIN", 6),
		ResponseMin:             l.integer("LIMITE_RESPOSTA_MIN", 12),
		BlocksMin:               l.integer("LIMITE_BLOCOS_MIN", 120),
		IPMin:                   l.integer("LIMITE_IP_MIN", 120),
		MaxConcurrentIngestions: l.integer("INGESTAO_MAX_SIMULTANEAS", 32),
		IngestionSlotWait:       l.dur("INGESTAO_ESPERA_VAGA", 100*time.Millisecond),
		RetryAfterOverload:      l.integer("RETRY_AFTER_SOBRECARGA", 5),
		RedisAddress:            l.str("REDIS_ENDERECO", ""),
		RedisPassword:           l.secret("REDIS_SENHA"),
		RedisTimeout:            l.dur("REDIS_TIMEOUT", 50*time.Millisecond),
	}
	c.Worker = WorkerConfig{
		Concurrency:        l.integer("WORKER_CONCORRENCIA", 4),
		PollInterval:       l.dur("WORKER_INTERVALO_POLL", 30*time.Second),
		ExpirationInterval: l.dur("WORKER_INTERVALO_EXPIRACAO", 60*time.Second),
		GroupWindow:        l.dur("ANCORA_JANELA_GRUPO", 10*time.Minute),
		MaxDriftPPM:        l.float("DERIVA_MAX_PPM", 40),
		SpanMin:            l.dur("ANCORA_SPAN_MIN", time.Hour),
		MinHalfWidthMs:     l.float("MEIA_LARGURA_MIN_MS", 0.5),
	}
	switch c.Environment {
	case "dev", "prodlike", "producao":
	default:
		l.errs = append(l.errs, "AMBIENTE deve ser dev, prodlike ou producao")
	}
	if c.Environment == "producao" && c.AllowNoneProfile {
		l.errs = append(l.errs, "PERMITIR_PERFIL_NONE é proibido em AMBIENTE=producao")
	}
	if c.Protocol.MaxBatchBlocks < 1 || c.Protocol.MaxBatchBlocks > 255 {
		l.errs = append(l.errs, "LOTE_MAX_BLOCOS fora de 1..255")
	}
	if len(l.errs) > 0 {
		return c, errors.New("configuração inválida: " + strings.Join(l.errs, "; "))
	}
	return c, nil
}

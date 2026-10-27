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
	Endereco          string
	MetricasEndereco  string
	SaudeWorker       string
	ProxiesConfiaveis []string
}

type BancoConfig struct {
	URL     string // DSN sem a senha
	Senha   string
	PoolMax int32
	PoolMin int32
}

// KMSConfig descreve o serviço KMS (módulo kms/), alcançado só por HTTP.
type KMSConfig struct {
	URL             string
	CACert          string
	ClientCert      string
	ClientKey       string
	Timeout         time.Duration
	MaxConcorrencia int
	VerifyLote      int
	VerifyParalelo  int
}

type JWTConfig struct {
	Emissor   string
	Audiencia string
	Validade  time.Duration
	Folga     time.Duration
}

type ProtocoloConfig struct {
	DesafioValidade     time.Duration
	DesafioMaxPendentes int
	AncoraRTTMax        time.Duration
	LoteMaxBlocos       int
	TLatSugerido        time.Duration
}

type LimitesConfig struct {
	DesafioMin, RespostaMin, BlocosMin, IPMin int
	IngestaoMaxSimultaneas                    int
	IngestaoEsperaVaga                        time.Duration
	RetryAfterSobrecarga                      int
	RedisEndereco                             string
	RedisSenha                                string
	RedisTimeout                              time.Duration
}

type WorkerConfig struct {
	Concorrencia       int
	IntervaloPoll      time.Duration
	IntervaloExpiracao time.Duration
	JanelaGrupo        time.Duration
	DerivaMaxPPM       float64
	SpanMin            time.Duration
	MeiaLarguraMinMs   float64
}

type Config struct {
	Ambiente           string
	LogNivel           string
	LogFormato         string
	HTTP               HTTPConfig
	Banco              BancoConfig
	KMS                KMSConfig
	JWT                JWTConfig
	Protocolo          ProtocoloConfig
	Limites            LimitesConfig
	Worker             WorkerConfig
	PermitirPerfilNone bool
}

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

// segredo lê NOME ou o conteúdo do arquivo apontado por NOME_ARQUIVO.
func (l *leitor) segredo(nome string) string {
	if arq := l.env(nome + "_ARQUIVO"); arq != "" {
		b, err := os.ReadFile(arq)
		if err != nil {
			l.errs = append(l.errs, fmt.Sprintf("%s_ARQUIVO: %v", nome, err))
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	return l.env(nome)
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

func (l *leitor) real(nome string, padrao float64) float64 {
	v := l.env(nome)
	if v == "" {
		return padrao
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: número inválido %q", nome, v))
		return padrao
	}
	return n
}

func (l *leitor) bool(nome string) bool {
	b, _ := strconv.ParseBool(l.env(nome))
	return b
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
	c.PermitirPerfilNone = l.bool("PERMITIR_PERFIL_NONE")

	c.HTTP = HTTPConfig{
		Endereco:         l.str("HTTP_ENDERECO", ":8080"),
		MetricasEndereco: l.str("METRICAS_ENDERECO", ":9090"),
		SaudeWorker:      l.str("SAUDE_ENDERECO_WORKER", ":8090"),
	}
	if p := l.str("PROXIES_CONFIAVEIS", ""); p != "" {
		for _, x := range strings.Split(p, ",") {
			c.HTTP.ProxiesConfiaveis = append(c.HTTP.ProxiesConfiaveis, strings.TrimSpace(x))
		}
	}
	c.Banco = BancoConfig{
		URL:     l.str("BANCO_URL", ""),
		Senha:   l.segredo("BANCO_SENHA"),
		PoolMax: int32(l.inteiro("BANCO_POOL_MAX", 40)),
		PoolMin: int32(l.inteiro("BANCO_POOL_MIN", 4)),
	}
	c.KMS = KMSConfig{
		URL:             l.str("KMS_URL", ""),
		CACert:          l.str("KMS_CACERT", ""),
		ClientCert:      l.str("KMS_CLIENT_CERT", ""),
		ClientKey:       l.str("KMS_CLIENT_KEY", ""),
		Timeout:         l.dur("KMS_TIMEOUT", 2*time.Second),
		MaxConcorrencia: l.inteiro("KMS_MAX_CONCORRENCIA", 32),
		VerifyLote:      l.inteiro("KMS_VERIFY_LOTE", 32),
		VerifyParalelo:  l.inteiro("KMS_VERIFY_PARALELO", 1),
	}
	c.JWT = JWTConfig{
		Emissor:   l.str("JWT_EMISSOR", "tccelta-api"),
		Audiencia: l.str("JWT_AUDIENCIA", "tccelta-ingestao"),
		Validade:  l.dur("JWT_VALIDADE", time.Hour),
		Folga:     l.dur("JWT_FOLGA", 30*time.Second),
	}
	c.Protocolo = ProtocoloConfig{
		DesafioValidade:     l.dur("DESAFIO_VALIDADE", 60*time.Second),
		DesafioMaxPendentes: l.inteiro("DESAFIO_MAX_PENDENTES", 3),
		AncoraRTTMax:        l.dur("ANCORA_RTT_MAX", 2*time.Second),
		LoteMaxBlocos:       l.inteiro("LOTE_MAX_BLOCOS", 32),
		TLatSugerido:        l.dur("T_LAT_SUGERIDO", 60*time.Second),
	}
	c.Limites = LimitesConfig{
		DesafioMin:             l.inteiro("LIMITE_DESAFIO_MIN", 6),
		RespostaMin:            l.inteiro("LIMITE_RESPOSTA_MIN", 12),
		BlocosMin:              l.inteiro("LIMITE_BLOCOS_MIN", 120),
		IPMin:                  l.inteiro("LIMITE_IP_MIN", 120),
		IngestaoMaxSimultaneas: l.inteiro("INGESTAO_MAX_SIMULTANEAS", 32),
		IngestaoEsperaVaga:     l.dur("INGESTAO_ESPERA_VAGA", 100*time.Millisecond),
		RetryAfterSobrecarga:   l.inteiro("RETRY_AFTER_SOBRECARGA", 5),
		RedisEndereco:          l.str("REDIS_ENDERECO", ""),
		RedisSenha:             l.segredo("REDIS_SENHA"),
		RedisTimeout:           l.dur("REDIS_TIMEOUT", 50*time.Millisecond),
	}
	c.Worker = WorkerConfig{
		Concorrencia:       l.inteiro("WORKER_CONCORRENCIA", 4),
		IntervaloPoll:      l.dur("WORKER_INTERVALO_POLL", 30*time.Second),
		IntervaloExpiracao: l.dur("WORKER_INTERVALO_EXPIRACAO", 60*time.Second),
		JanelaGrupo:        l.dur("ANCORA_JANELA_GRUPO", 10*time.Minute),
		DerivaMaxPPM:       l.real("DERIVA_MAX_PPM", 40),
		SpanMin:            l.dur("ANCORA_SPAN_MIN", time.Hour),
		MeiaLarguraMinMs:   l.real("MEIA_LARGURA_MIN_MS", 0.5),
	}
	switch c.Ambiente {
	case "dev", "prodlike", "producao":
	default:
		l.errs = append(l.errs, "AMBIENTE deve ser dev, prodlike ou producao")
	}
	if c.Ambiente == "producao" && c.PermitirPerfilNone {
		l.errs = append(l.errs, "PERMITIR_PERFIL_NONE é proibido em AMBIENTE=producao")
	}
	if c.Protocolo.LoteMaxBlocos < 1 || c.Protocolo.LoteMaxBlocos > 255 {
		l.errs = append(l.errs, "LOTE_MAX_BLOCOS fora de 1..255")
	}
	if len(l.errs) > 0 {
		return c, errors.New("configuração inválida: " + strings.Join(l.errs, "; "))
	}
	return c, nil
}

// Package ocorrencia define lacunas e alertas, os registros de anomalia do dongle.
package ocorrencia

import "time"

// Causas de lacuna.
const (
	CausaOverflow = "OVERFLOW"
	CausaBoot     = "BOOT"
	CausaLoss     = "LOSS"
)

// Tipos de alerta.
const (
	AlertaRelogioRegrediu = "relogio_servidor_regrediu"
	AlertaPIDDesconhecido = "pid_desconhecido"
	AlertaInvariante      = "invariante_violada"
	AlertaGeracaoAvancou  = "geracao_boot_avancou"
)

// Lacuna é um intervalo de seq perdido. CS (handshake) e SeqBloco (ingestão) são opcionais.
type Lacuna struct {
	DongleID     int32
	From, To     int64
	Causa        string
	RegistradoEm time.Time
	CS           *int64
	SeqBloco     *int64
}

// Alerta é um evento a investigar. Detalhe é JSON.
type Alerta struct {
	DongleID    int32
	GeracaoBoot *int32
	BootID      *int32
	Tipo        string
	Detalhe     []byte
}

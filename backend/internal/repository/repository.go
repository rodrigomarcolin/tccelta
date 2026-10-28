// Package repositorio é a porta do banco: interfaces por agregado, só com tipos de domínio.
// Os adaptadores ficam em repositorio/postgres; o decorator de cache em repositorio/cache.
package repositorio

import (
	"context"
	"time"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/dominio/dongle"
	"tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/ocorrencia"
	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/dominio/tempo"
)

// ErrBancoOcupado: lock_timeout, deadlock ou falha de serialização persistente (-> 503).
var ErrBancoOcupado = porta.ErrBancoOcupado

// Dongles é o cadastro. BuscarPorDevID devolve dongle.ErrDesconhecido quando não há cadastro.
type Dongles interface {
	BuscarPorDevID(ctx context.Context, dev protocolo.DevID) (dongle.Dongle, error)
	// Travar toma o lock de aconselhamento do dongle até o fim da transação.
	Travar(ctx context.Context, dongleID int32) error
}

// Desafios guarda o ciclo de vida dos desafios do handshake.
type Desafios interface {
	// Emitir devolve handshake.ErrDesafiosPendentes quando o dongle já tem pendentes demais.
	Emitir(ctx context.Context, e handshake.Emissao) (handshake.Desafio, error)
	// Buscar devolve handshake.ErrDesafioInvalido quando o desafio não existe.
	Buscar(ctx context.Context, dongleID int32, cs int64) (handshake.DesafioGravado, error)
	// Consumir devolve S1 do desafio; handshake.ErrDesafioInvalido se não estiver pendente e no prazo.
	Consumir(ctx context.Context, c handshake.Consumo) (s1 time.Time, err error)
	Finalizar(ctx context.Context, f handshake.Finalizacao) error
	MarcarTagInvalida(ctx context.Context, dongleID int32, cs int64, nonce []byte) error
}

// Estados é dongle_estado.
type Estados interface {
	Ler(ctx context.Context, dongleID int32) (dongle.Estado, error)
	// LerSnapshot lê SeqContiguo, CConf e UltimoAck.
	LerSnapshot(ctx context.Context, dongleID int32) (dongle.Estado, error)
	LerUltimoAck(ctx context.Context, dongleID int32) ([]byte, error)
	AtualizarHandshake(ctx context.Context, a handshake.Atualizacao) error
	AtualizarIngestao(ctx context.Context, a ingestao.AtualizacaoEstado) error
}

// Boots é a tabela de boots (tempo por boot_id e âncoras).
type Boots interface {
	// TMsUltimo devolve as referências candidatas nas gerações gMin e gMax para os boot_ids dados.
	TMsUltimo(ctx context.Context, dongleID int32, bootIDs []int32, gMin, gMax int32) ([]tempo.RefBoot, error)
	GarantirPendente(ctx context.Context, dongleID, geracao, bootID int32, agora time.Time) error
	AtualizarIngestao(ctx context.Context, dongleID int32, agora time.Time, agregados []ingestao.AgregadoBoot) error
	InserirAncora(ctx context.Context, a handshake.Ancora) error
	NotificarPendente(ctx context.Context, chave string) error
}

// Blocos é bronze.bloco_bruto, os rejeitados e as linhas derivadas (silver).
type Blocos interface {
	// InserirBrutos devolve quantos blocos foram gravados (os repetidos não contam).
	InserirBrutos(ctx context.Context, dongleID int32, epoca int64, recebidoEm time.Time, blocos []ingestao.BlocoBruto) (int, error)
	RegistrarRejeitado(ctx context.Context, r ingestao.Rejeitado) error
	CopiarLeituras(ctx context.Context, dongleID int32, recebidoEm time.Time, ls []pid.Leitura) error
	CopiarPosicoes(ctx context.Context, dongleID int32, recebidoEm time.Time, ps []pid.Posicao) error
}

// Ocorrencias são lacunas e alertas.
type Ocorrencias interface {
	InserirLacuna(ctx context.Context, l ocorrencia.Lacuna) error
	InserirAlerta(ctx context.Context, a ocorrencia.Alerta) error
}

// Pids é o catálogo de PIDs.
type Pids interface {
	Listar(ctx context.Context) ([]pid.Def, error)
}

// Repositorios agrupa os repositórios; numa transação, todos enxergam a mesma tx.
type Repositorios struct {
	Dongles     Dongles
	Desafios    Desafios
	Estados     Estados
	Boots       Boots
	Blocos      Blocos
	Ocorrencias Ocorrencias
	Pids        Pids
}

// Transacionador executa fn numa transação READ COMMITTED com timeouts locais. Deadlock e falha de
// serialização repetem fn uma vez; o que sobrar vira ErrBancoOcupado. Erro de fn desfaz a transação.
type Transacionador interface {
	EmTransacao(ctx context.Context, fn func(ctx context.Context, r Repositorios) error) error
}

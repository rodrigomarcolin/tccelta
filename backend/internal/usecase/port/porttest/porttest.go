// Package portateste tem dublês das portas compartilhadas, para testes e para o simulador.
package portateste

import (
	"time"

	"tccelta/backend/internal/casodeuso/porta"
)

// Nop descarta todas as métricas; implementa todas as portas de métricas.
type Nop struct{}

func (Nop) Handshake(string)                                 {}
func (Nop) Lacuna(string)                                    {}
func (Nop) AncoraRTT(time.Duration)                          {}
func (Nop) Blocos(string, int)                               {}
func (Nop) Lote(string)                                      {}
func (Nop) DuracaoTxIngestao(time.Duration)                  {}
func (Nop) Linhas(string, int)                               {}
func (Nop) Requisicao(string, int, time.Duration)            {}
func (Nop) Rejeicao(string)                                  {}
func (Nop) ChamadaKMS(string, string, string, time.Duration) {}
func (Nop) DisjuntorKMS(bool)                                {}

var (
	_ porta.MetricasHandshake = Nop{}
	_ porta.MetricasIngestao  = Nop{}
	_ porta.MetricasHTTP      = Nop{}
	_ porta.MetricasKMS       = Nop{}
)

// RelogioFixo devolve um relógio que sempre responde t. Para avançar, use RelogioAjustavel.
func RelogioFixo(t time.Time) porta.Relogio { return func() time.Time { return t } }

// RelogioAjustavel é um relógio controlado pelo teste.
type RelogioAjustavel struct{ T time.Time }

// Agora implementa porta.Relogio via método (use r.Agora como valor).
func (r *RelogioAjustavel) Agora() time.Time { return r.T }

// Avancar soma d ao relógio.
func (r *RelogioAjustavel) Avancar(d time.Duration) { r.T = r.T.Add(d) }

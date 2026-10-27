// Package dongles mantém o cache DevId -> dongle (TTL).
package dongles

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/protocolo"
)

// ErrDesconhecido: DevId sem cadastro.
var ErrDesconhecido = errors.New("dongle_desconhecido")

// Dongle é a linha de cadastro usada pelo caminho quente.
type Dongle struct {
	ID           int32
	DevID        protocolo.DevID
	Perfil       protocolo.Perfil
	ChaveAuth    string
	ChaveRec     string
	ChavePublica []byte
	Ativo        bool
}

type entrada struct {
	d   Dongle
	ate time.Time
}

// Cache guarda dongles por DevId.
type Cache struct {
	q     consultas.Querier
	ttl   time.Duration
	max   int
	agora func() time.Time
	mu    sync.Mutex
	m     map[protocolo.DevID]entrada
}

// Novo cria o cache (padrão: 10 mil entradas, TTL 60 s).
func Novo(q consultas.Querier, max int, ttl time.Duration) *Cache {
	if max <= 0 {
		max = 10000
	}
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &Cache{q: q, ttl: ttl, max: max, agora: time.Now, m: map[protocolo.DevID]entrada{}}
}

// Buscar devolve o dongle (do cache ou do banco).
func (c *Cache) Buscar(ctx context.Context, dev protocolo.DevID) (Dongle, error) {
	now := c.agora()
	c.mu.Lock()
	if e, ok := c.m[dev]; ok && now.Before(e.ate) {
		c.mu.Unlock()
		return e.d, nil
	}
	c.mu.Unlock()
	r, err := c.q.BuscarDonglePorDevId(ctx, dev[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Dongle{}, ErrDesconhecido
		}
		return Dongle{}, err
	}
	p, err := protocolo.PerfilPorNome(r.PerfilSeguranca)
	if err != nil {
		return Dongle{}, err
	}
	d := Dongle{ID: r.DongleID, DevID: dev, Perfil: p, ChaveAuth: r.KmsChaveAuth, ChaveRec: r.KmsChaveRec,
		ChavePublica: r.ChavePublica, Ativo: r.Ativo}
	c.mu.Lock()
	if len(c.m) >= c.max {
		c.m = map[protocolo.DevID]entrada{}
	}
	c.m[dev] = entrada{d: d, ate: now.Add(c.ttl)}
	c.mu.Unlock()
	return d, nil
}

// Invalidar remove um DevId do cache.
func (c *Cache) Invalidar(dev protocolo.DevID) {
	c.mu.Lock()
	delete(c.m, dev)
	c.mu.Unlock()
}

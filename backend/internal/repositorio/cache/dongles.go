// Package cache tem o decorator com TTL do repositório de dongles (DevId -> dongle).
package cache

import (
	"context"
	"sync"
	"time"

	"tccelta/backend/internal/dominio/dongle"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/repositorio"
)

type entrada struct {
	d   dongle.Dongle
	ate time.Time
}

// Dongles guarda dongles por DevId na frente de outro repositorio.Dongles.
type Dongles struct {
	repositorio.Dongles
	ttl   time.Duration
	max   int
	agora func() time.Time
	mu    sync.Mutex
	m     map[protocolo.DevID]entrada
}

// NovoDongles cria o cache (padrão: 10 mil entradas, TTL 60 s).
func NovoDongles(base repositorio.Dongles, max int, ttl time.Duration) *Dongles {
	if max <= 0 {
		max = 10000
	}
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &Dongles{Dongles: base, ttl: ttl, max: max, agora: time.Now, m: map[protocolo.DevID]entrada{}}
}

// BuscarPorDevID devolve o dongle do cache ou do repositório de baixo.
func (c *Dongles) BuscarPorDevID(ctx context.Context, dev protocolo.DevID) (dongle.Dongle, error) {
	now := c.agora()
	c.mu.Lock()
	if e, ok := c.m[dev]; ok && now.Before(e.ate) {
		c.mu.Unlock()
		return e.d, nil
	}
	c.mu.Unlock()
	d, err := c.Dongles.BuscarPorDevID(ctx, dev)
	if err != nil {
		return dongle.Dongle{}, err
	}
	c.mu.Lock()
	if len(c.m) >= c.max {
		c.m = map[protocolo.DevID]entrada{}
	}
	c.m[dev] = entrada{d: d, ate: now.Add(c.ttl)}
	c.mu.Unlock()
	return d, nil
}

// Invalidar remove um DevId do cache.
func (c *Dongles) Invalidar(dev protocolo.DevID) {
	c.mu.Lock()
	delete(c.m, dev)
	c.mu.Unlock()
}

var _ repositorio.Dongles = (*Dongles)(nil)

// Package cache tem o decorator com TTL do repositório de dongles (DevId -> dongle).
package cache

import (
	"context"
	"sync"
	"time"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
)

type input struct {
	d     dongle.Dongle
	until time.Time
}

// Dongles guarda dongles por DevId na frente de outro port.Dongles.
type Dongles struct {
	port.Dongles
	ttl time.Duration
	max int
	now func() time.Time
	mu  sync.Mutex
	m   map[protocol.DevID]input
}

// NewDongles cria o cache (padrão: 10 mil entradas, TTL 60 s).
func NewDongles(base port.Dongles, max int, ttl time.Duration) *Dongles {
	if max <= 0 {
		max = 10000
	}
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &Dongles{Dongles: base, ttl: ttl, max: max, now: time.Now, m: map[protocol.DevID]input{}}
}

// FindByDevID devolve o dongle do cache ou do repositório de baixo.
func (c *Dongles) FindByDevID(ctx context.Context, dev protocol.DevID) (dongle.Dongle, error) {
	now := c.now()
	c.mu.Lock()
	if e, ok := c.m[dev]; ok && now.Before(e.until) {
		c.mu.Unlock()
		return e.d, nil
	}
	c.mu.Unlock()
	d, err := c.Dongles.FindByDevID(ctx, dev)
	if err != nil {
		return dongle.Dongle{}, err
	}
	c.mu.Lock()
	if len(c.m) >= c.max {
		c.m = map[protocol.DevID]input{}
	}
	c.m[dev] = input{d: d, until: now.Add(c.ttl)}
	c.mu.Unlock()
	return d, nil
}

// Invalidate remove um DevId do cache.
func (c *Dongles) Invalidate(dev protocol.DevID) {
	c.mu.Lock()
	delete(c.m, dev)
	c.mu.Unlock()
}

var _ port.Dongles = (*Dongles)(nil)

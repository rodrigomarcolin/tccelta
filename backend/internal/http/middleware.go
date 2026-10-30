package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/port"
)

type ctxKey int

const (
	ctxS2 ctxKey = iota
	ctxSession
	ctxReqID
)

type respWriter struct {
	http.ResponseWriter
	status int
}

func (r *respWriter) WriteHeader(s int) { r.status = s; r.ResponseWriter.WriteHeader(s) }

type mw = func(http.Handler) http.Handler

func recoverPanics(log *slog.Logger) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if x := recover(); x != nil {
					log.Error("http.panic", "erro", fmt.Sprint(x), "pilha", string(debug.Stack()))
					problem(w, 500, "erro_interno", "erro interno", 0)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// instrument: X-Request-Id, log de acesso e métricas HTTP.
func (d *Deps) instrument(route string) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t0 := time.Now()
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = uuid.NewString()
			}
			w.Header().Set("X-Request-Id", id)
			rw := &respWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), ctxReqID, id)))
			dur := time.Since(t0)
			d.Met.Request(route, rw.status, dur)
			d.Log.Info("http.acesso", "req_id", id, "rota", route, "status", rw.status, "duracao_ms", dur.Milliseconds())
		})
	}
}

// annotateS2 guarda o instante de chegada antes de qualquer leitura de corpo.
func annotateS2(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxS2, time.Now())))
	})
}

func (d *Deps) ipFrom(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		for _, c := range d.Cfg.HTTP.TrustedProxies {
			if p, err := netip.ParsePrefix(c); err == nil && p.Contains(ip) {
				if x := strings.TrimSpace(r.Header.Get("X-Real-IP")); x != "" {
					return x
				}
			}
		}
	}
	return host
}

func (d *Deps) limitIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, ra := d.Limiter.Allow(r.Context(), "ip", d.ipFrom(r), d.Cfg.Limits.IPMin); !ok {
			d.rej("rate_limit")
			problem(w, 429, "limite_taxa", "limite por IP", ra)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d *Deps) limitDevID(scope string, limit int) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.PathValue("id")
			if s, ok := r.Context().Value(ctxSession).(port.Session); ok {
				key = s.Dev.Hex()
			}
			if ok, ra := d.Limiter.Allow(r.Context(), scope, strings.ToLower(key), limit); !ok {
				d.rej("rate_limit")
				problem(w, 429, "limite_taxa", "limite por dongle", ra)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (d *Deps) limitConcurrency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lib, ok := d.Semaphore.Acquire()
		if !ok {
			d.rej("overload")
			problem(w, 503, "sobrecarga", "servidor ocupado", d.retry503())
			return
		}
		defer lib()
		next.ServeHTTP(w, r)
	})
}

func limitBody(max int64) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

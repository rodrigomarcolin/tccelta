package limits

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestLocal(t *testing.T) {
	now := time.Unix(600, 0)
	l := NewLocal()
	l.Now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow(context.Background(), "e", "k", 3); !ok {
			t.Fatal("deveria permitir")
		}
	}
	ok, ra := l.Allow(context.Background(), "e", "k", 3)
	if ok || ra < time.Second {
		t.Fatal("deveria bloquear", ra)
	}
	now = now.Add(time.Minute)
	if ok, _ := l.Allow(context.Background(), "e", "k", 3); !ok {
		t.Fatal("janela nova")
	}
}

func TestSharedRedis(t *testing.T) {
	mr := miniredis.RunT(t)
	c1 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	a, b := NewRedis(c1, time.Second), NewRedis(c2, time.Second)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if ok, _ := a.Allow(ctx, "e", "k", 4); !ok {
			t.Fatal()
		}
		if ok, _ := b.Allow(ctx, "e", "k", 4); !ok {
			t.Fatal()
		}
	}
	if ok, ra := a.Allow(ctx, "e", "k", 4); ok || ra < time.Second {
		t.Fatal("as duas réplicas somam no mesmo limite")
	}
}

func TestFallbackOpensBreaker(t *testing.T) {
	mr := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	failures := 0
	l := &LimiterWithFallback{Local: NewLocal(), Redis: NewRedis(c, 100*time.Millisecond), OnFailure: func() { failures++ }}
	mr.Close()
	for i := 0; i < 8; i++ {
		if ok, _ := l.Allow(context.Background(), "e", "k"+string(rune('a'+i)), 10); !ok {
			t.Fatal("redis fora nunca bloqueia")
		}
	}
	if failures != 5 {
		t.Fatalf("falhas = %d, o disjuntor deveria abrir após 5", failures)
	}
}

func TestConcurrency(t *testing.T) {
	c := NewConcurrency(1, 20*time.Millisecond)
	lib, ok := c.Acquire()
	if !ok {
		t.Fatal()
	}
	if _, ok := c.Acquire(); ok {
		t.Fatal("cheio")
	}
	lib()
	if _, ok := c.Acquire(); !ok {
		t.Fatal("liberou")
	}
}

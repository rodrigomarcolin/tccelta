package limites

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestLocal(t *testing.T) {
	agora := time.Unix(600, 0)
	l := NovoLocal()
	l.Agora = func() time.Time { return agora }
	for i := 0; i < 3; i++ {
		if ok, _ := l.Permitir(context.Background(), "e", "k", 3); !ok {
			t.Fatal("deveria permitir")
		}
	}
	ok, ra := l.Permitir(context.Background(), "e", "k", 3)
	if ok || ra < time.Second {
		t.Fatal("deveria bloquear", ra)
	}
	agora = agora.Add(time.Minute)
	if ok, _ := l.Permitir(context.Background(), "e", "k", 3); !ok {
		t.Fatal("janela nova")
	}
}

func TestRedisCompartilhado(t *testing.T) {
	mr := miniredis.RunT(t)
	c1 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	a, b := NovoRedis(c1, time.Second), NovoRedis(c2, time.Second)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if ok, _ := a.Permitir(ctx, "e", "k", 4); !ok {
			t.Fatal()
		}
		if ok, _ := b.Permitir(ctx, "e", "k", 4); !ok {
			t.Fatal()
		}
	}
	if ok, ra := a.Permitir(ctx, "e", "k", 4); ok || ra < time.Second {
		t.Fatal("as duas réplicas somam no mesmo limite")
	}
}

func TestFallbackAbreDisjuntor(t *testing.T) {
	mr := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	falhas := 0
	l := &LimitadorComFallback{Local: NovoLocal(), Redis: NovoRedis(c, 100*time.Millisecond), OnFalha: func() { falhas++ }}
	mr.Close()
	for i := 0; i < 8; i++ {
		if ok, _ := l.Permitir(context.Background(), "e", "k"+string(rune('a'+i)), 10); !ok {
			t.Fatal("redis fora nunca bloqueia")
		}
	}
	if falhas != 5 {
		t.Fatalf("falhas = %d, o disjuntor deveria abrir após 5", falhas)
	}
}

func TestConcorrencia(t *testing.T) {
	c := NovaConcorrencia(1, 20*time.Millisecond)
	lib, ok := c.Adquirir()
	if !ok {
		t.Fatal()
	}
	if _, ok := c.Adquirir(); ok {
		t.Fatal("cheio")
	}
	lib()
	if _, ok := c.Adquirir(); !ok {
		t.Fatal("liberou")
	}
}

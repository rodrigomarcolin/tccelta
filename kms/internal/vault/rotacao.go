package kms

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"
)

// Rotacionador faz o ciclo de vida da chave do JWT: rotaciona após Periodo e expurga a versão k
// quando nenhum token assinado com ela pode mais estar válido.
type Rotacionador struct {
	T       Transporte
	Mount   string
	Chave   string
	Periodo time.Duration
	// Validade = JWT_VALIDADE + JWT_FOLGA + 10 min de margem.
	Validade time.Duration
	Agora    func() time.Time
	Log      *slog.Logger
}

type versaoChave struct {
	n      int
	criada time.Time
}

func (r *Rotacionador) agora() time.Time {
	if r.Agora != nil {
		return r.Agora()
	}
	return time.Now()
}

func (r *Rotacionador) estado(ctx context.Context) (versoes []versaoChave, minDec int, err error) {
	resp, err := r.T.Ler(ctx, fmt.Sprintf("%s/keys/%s", r.Mount, r.Chave))
	if err != nil {
		return nil, 0, err
	}
	d, _ := resp["data"].(map[string]any)
	minDec = 1
	if f, ok := d["min_decryption_version"].(float64); ok && f >= 1 {
		minDec = int(f)
	}
	ks, _ := d["keys"].(map[string]any)
	for k, x := range ks {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		m, _ := x.(map[string]any)
		cs, _ := m["creation_time"].(string)
		t, err := time.Parse(time.RFC3339Nano, cs)
		if err != nil {
			return nil, 0, fmt.Errorf("creation_time da v%d: %w", n, err)
		}
		versoes = append(versoes, versaoChave{n, t})
	}
	sort.Slice(versoes, func(i, j int) bool { return versoes[i].n < versoes[j].n })
	if len(versoes) == 0 {
		return nil, 0, fmt.Errorf("chave %s sem versões", r.Chave)
	}
	return versoes, minDec, nil
}

// Verificar roda um ciclo: rotaciona se vencido e expurga as versões antigas. Idempotente.
func (r *Rotacionador) Verificar(ctx context.Context, forcar bool) (rotacionou bool, minNovo int, err error) {
	vs, minDec, err := r.estado(ctx)
	if err != nil {
		return false, 0, err
	}
	ultima := vs[len(vs)-1]
	if forcar || r.agora().Sub(ultima.criada) >= r.Periodo {
		if _, err := r.T.Escrever(ctx, fmt.Sprintf("%s/keys/%s/rotate", r.Mount, r.Chave), nil); err != nil {
			return false, 0, err
		}
		rotacionou = true
		if vs, minDec, err = r.estado(ctx); err != nil {
			return true, 0, err
		}
	}
	// expurgo: versão k sai quando agora >= criação(k+1) + validade
	novoMin := minDec
	for i := 0; i+1 < len(vs); i++ {
		if vs[i].n < novoMin {
			continue
		}
		if !r.agora().Before(vs[i+1].criada.Add(r.Validade)) {
			novoMin = vs[i+1].n
		} else {
			break
		}
	}
	if novoMin > minDec {
		if _, err := r.T.Escrever(ctx, fmt.Sprintf("%s/keys/%s/config", r.Mount, r.Chave),
			map[string]any{"min_decryption_version": novoMin}); err != nil {
			return rotacionou, minDec, err
		}
		if r.Log != nil {
			r.Log.Info("jwt.expurgo", "min_decryption_version", novoMin)
		}
		return rotacionou, novoMin, nil
	}
	return rotacionou, minDec, nil
}

// Agendar roda Verificar a cada intervalo até o contexto terminar.
func (r *Rotacionador) Agendar(ctx context.Context, cada time.Duration) {
	for {
		if rot, min, err := r.Verificar(ctx, false); err != nil {
			r.Log.Error("jwt.rotacao_falhou", "erro", err)
		} else if rot {
			r.Log.Info("jwt.rotacionada", "min_decryption_version", min)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(cada):
		}
	}
}

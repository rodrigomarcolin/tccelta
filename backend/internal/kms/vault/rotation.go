package vault

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"
)

// Rotator faz o ciclo de vida da chave do JWT: rotaciona após Periodo e expurga a versão k
// quando nenhum token assinado com ela pode mais estar válido.
type Rotator struct {
	T      Transport
	Mount  string
	Key    string
	Period time.Duration
	// Validity = JWT_VALIDADE + JWT_FOLGA + 10 min de margem.
	Validity time.Duration
	Now      func() time.Time
	Log      *slog.Logger
}

type keyVersion struct {
	n       int
	created time.Time
}

func (r *Rotator) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Rotator) state(ctx context.Context) (versions []keyVersion, minDec int, err error) {
	resp, err := r.T.Read(ctx, fmt.Sprintf("%s/keys/%s", r.Mount, r.Key))
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
		versions = append(versions, keyVersion{n, t})
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].n < versions[j].n })
	if len(versions) == 0 {
		return nil, 0, fmt.Errorf("chave %s sem versões", r.Key)
	}
	return versions, minDec, nil
}

// Check roda um ciclo: rotaciona se vencido e expurga as versões antigas. Idempotente.
func (r *Rotator) Check(ctx context.Context, force bool) (rotated bool, minAfter int, err error) {
	vs, minDec, err := r.state(ctx)
	if err != nil {
		return false, 0, err
	}
	last := vs[len(vs)-1]
	if force || r.now().Sub(last.created) >= r.Period {
		if _, err := r.T.Write(ctx, fmt.Sprintf("%s/keys/%s/rotate", r.Mount, r.Key), nil); err != nil {
			return false, 0, err
		}
		rotated = true
		if vs, minDec, err = r.state(ctx); err != nil {
			return true, 0, err
		}
	}
	// expurgo: versão k sai quando agora >= criação(k+1) + validade
	newMin := minDec
	for i := 0; i+1 < len(vs); i++ {
		if vs[i].n < newMin {
			continue
		}
		if !r.now().Before(vs[i+1].created.Add(r.Validity)) {
			newMin = vs[i+1].n
		} else {
			break
		}
	}
	if newMin > minDec {
		if _, err := r.T.Write(ctx, fmt.Sprintf("%s/keys/%s/config", r.Mount, r.Key),
			map[string]any{"min_decryption_version": newMin}); err != nil {
			return rotated, minDec, err
		}
		if r.Log != nil {
			r.Log.Info("jwt.expurgo", "min_decryption_version", newMin)
		}
		return rotated, newMin, nil
	}
	return rotated, minDec, nil
}

// Schedule roda Verificar a cada intervalo até o contexto terminar.
func (r *Rotator) Schedule(ctx context.Context, every time.Duration) {
	for {
		if rot, min, err := r.Check(ctx, false); err != nil {
			r.Log.Error("jwt.rotacao_falhou", "erro", err)
		} else if rot {
			r.Log.Info("jwt.rotacionada", "min_decryption_version", min)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

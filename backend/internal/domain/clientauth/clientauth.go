// Package clientauth tem as regras puras do client credentials: o formato do client_id e do segredo e a progressão do
// bloqueio por tentativas de segredo errado. O armazenamento do estado do bloqueio está nas portas (internal/port).
package clientauth

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"
)

// MaxSecretLen é o tamanho máximo aceito para o segredo (texto; o gerado tem 43 caracteres).
const MaxSecretLen = 128

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,63}$`)

// ValidID diz se o client_id tem o formato aceito.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// ValidSecret diz se o segredo tem de 1 a MaxSecretLen caracteres.
func ValidSecret(s string) bool { return len(s) >= 1 && len(s) <= MaxSecretLen }

// ErrInvalidClient: sem credenciais, cliente desconhecido, revogado ou segredo errado (indistinguíveis de propósito).
var ErrInvalidClient = errors.New("cliente_invalido")

// ErrBlocked: o par (client_id, IP) ou o IP está bloqueado por falhas seguidas.
var ErrBlocked = errors.New("bloqueado")

// BlockedError diz por quanto tempo o bloqueio ainda vale. Is(ErrBlocked) é verdadeiro.
type BlockedError struct{ RetryAfter time.Duration }

func (e BlockedError) Error() string {
	return fmt.Sprintf("bloqueado por mais %s", e.RetryAfter.Round(time.Second))
}

// Is permite errors.Is(err, ErrBlocked).
func (e BlockedError) Is(target error) bool { return target == ErrBlocked }

// Policy é a progressão do bloqueio de um contador. Passada a tolerância T, a falha de número n bloqueia por
// min(Initial × Factor^(n−T−1), Cap). Sem falha por Forget, o contador é esquecido.
type Policy struct {
	Tolerance int
	Initial   time.Duration
	Factor    float64
	Cap       time.Duration
	Forget    time.Duration
}

// Block devolve o tempo de bloqueio causado pela falha de número n (1 para a primeira); zero dentro da tolerância.
func (p Policy) Block(n int) time.Duration {
	if n <= p.Tolerance {
		return 0
	}
	d := float64(p.Initial) * math.Pow(p.Factor, float64(n-p.Tolerance-1))
	if d >= float64(p.Cap) || math.IsInf(d, 0) || math.IsNaN(d) {
		return p.Cap
	}
	return time.Duration(d)
}

// Validate recusa uma política que não progride: tolerância < 1, fator < 1 ou teto < inicial.
func (p Policy) Validate() error {
	switch {
	case p.Tolerance < 1:
		return errors.New("a tolerância deve ser pelo menos 1")
	case p.Factor < 1:
		return errors.New("o fator deve ser pelo menos 1")
	case p.Initial <= 0 || p.Cap < p.Initial:
		return errors.New("o teto deve ser maior ou igual ao tempo inicial, que deve ser positivo")
	case p.Forget <= 0:
		return errors.New("o tempo de esquecimento deve ser positivo")
	}
	return nil
}

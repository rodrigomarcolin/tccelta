package port

import "time"

// Clock devolve o instante atual.
type Clock func() time.Time

// RealClock é o relógio do sistema.
func RealClock() time.Time { return time.Now() }

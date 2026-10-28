package porta

import "time"

// Relogio devolve o instante atual.
type Relogio func() time.Time

// RelogioReal é o relógio do sistema.
func RelogioReal() time.Time { return time.Now() }

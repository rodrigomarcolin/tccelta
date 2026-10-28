package port

import "errors"

// ErrDatabaseBusy: lock_timeout, deadlock ou falha de serialização persistente no banco (-> 503).
// Os adaptadores de banco o devolvem; o transporte HTTP o mapeia sem importar adaptadores.
var ErrDatabaseBusy = errors.New("banco_ocupado")

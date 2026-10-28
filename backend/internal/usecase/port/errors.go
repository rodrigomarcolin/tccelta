package porta

import "errors"

// ErrBancoOcupado: lock_timeout, deadlock ou falha de serialização persistente no banco (-> 503).
// Os repositórios o devolvem; fica aqui para o transporte HTTP poder mapeá-lo sem importar repositórios.
var ErrBancoOcupado = errors.New("banco_ocupado")

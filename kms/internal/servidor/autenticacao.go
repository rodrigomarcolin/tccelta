package servidor

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"os"
)

// ConfigTLS monta o tls.Config do servidor com mTLS obrigatório: só aceita clientes com certificado
// assinado pela CA indicada.
func ConfigTLS(cert, chave, caCliente string) (*tls.Config, error) {
	par, err := tls.LoadX509KeyPair(cert, chave)
	if err != nil {
		return nil, err
	}
	pem, err := os.ReadFile(caCliente)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("KMS_TLS_CLIENT_CA: PEM inválido")
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{par},
		ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}, nil
}

// exigirCliente recusa o que chegar sem certificado de cliente verificado. O tls.Config já impede
// isso na conexão; este middleware garante o mesmo se alguém servir o roteador sem mTLS por engano.
func (s *Servidor) exigirCliente(next http.Handler) http.Handler {
	if !s.ExigirCertCliente {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			if s.Met != nil {
				s.Met.Rejeicao("sem_cert_cliente")
			}
			problema(w, 401, "cliente_nao_autenticado", "certificado de cliente (mTLS) obrigatório", 0)
			return
		}
		next.ServeHTTP(w, r)
	})
}

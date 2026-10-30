package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"os"
)

// ConfigTLS monta o tls.Config do servidor com mTLS obrigatório: só aceita clientes com certificado
// assinado pela CA indicada.
func ConfigTLS(cert, key, caClient string) (*tls.Config, error) {
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	pem, err := os.ReadFile(caClient)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("KMS_TLS_CLIENT_CA: PEM inválido")
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair},
		ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}, nil
}

// requireClient recusa o que chegar sem certificado de cliente verificado. O tls.Config já impede
// isso na conexão; este middleware garante o mesmo se alguém servir o roteador sem mTLS por engano.
func (s *Server) requireClient(next http.Handler) http.Handler {
	if !s.RequireClientCert {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			if s.Metrics != nil {
				s.Metrics.Reject("sem_cert_cliente")
			}
			problem(w, 401, "cliente_nao_autenticado", "certificado de cliente (mTLS) obrigatório", 0)
			return
		}
		// a CA pode ter emitido outros certificados de cliente: só o da API (CN configurado) passa
		if r.TLS.VerifiedChains[0][0].Subject.CommonName != s.ClientCN {
			if s.Metrics != nil {
				s.Metrics.Reject("cliente_nao_autorizado")
			}
			problem(w, 403, "cliente_nao_autorizado", "certificado de cliente não autorizado", 0)
			return
		}
		next.ServeHTTP(w, r)
	})
}

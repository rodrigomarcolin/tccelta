package vault

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Registro de cliente (client credentials) no KV v2 do Vault. O Vault guarda só o hash do segredo.
const (
	// ClientAlgo identifica o esquema de hash do registro.
	ClientAlgo = "sha256-v1"
	// ClientActive e ClientRevoked são os valores de ClientRecord.Status.
	ClientActive  = "active"
	ClientRevoked = "revoked"
	// SecretBytes é o tamanho do segredo gerado (256 bits); SaltBytes, o do sal por cliente.
	SecretBytes = 32
	SaltBytes   = 16
)

// ErrClientExists: já existe registro para o client_id (create usa cas=0).
var ErrClientExists = errors.New("cliente já existe")

// ClientRecord é o conteúdo de kv-clientes/data/clients/<client_id>.
type ClientRecord struct {
	Algo      string
	Salt      string // base64 (16 B); vazio quando revogado
	Hash      string // base64 (32 B); vazio quando revogado
	Status    string
	CreatedAt string
}

// SecretHash calcula SHA-256(salt ‖ secret). O segredo tem 256 bits de entropia, então um hash rápido basta.
func SecretHash(salt []byte, secret string) []byte {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(secret))
	return h.Sum(nil)
}

// NewSecret gera um segredo de 256 bits em base64url sem padding (43 caracteres) e o registro com sal novo.
func NewSecret(now time.Time) (secret string, rec ClientRecord, err error) {
	raw := make([]byte, SecretBytes)
	salt := make([]byte, SaltBytes)
	if _, err = rand.Read(raw); err != nil {
		return "", ClientRecord{}, err
	}
	if _, err = rand.Read(salt); err != nil {
		return "", ClientRecord{}, err
	}
	secret = base64.RawURLEncoding.EncodeToString(raw)
	return secret, ClientRecord{Algo: ClientAlgo, Salt: b64(salt), Hash: b64(SecretHash(salt, secret)), Status: ClientActive,
		CreatedAt: now.UTC().Format(time.RFC3339)}, nil
}

// Registro-fantasma: cliente inexistente ou revogado compara contra ele, para executar o mesmo trabalho.
var (
	ghostSalt = []byte("tccelta-fantasma")
	ghostHash = SecretHash(ghostSalt, "tccelta-registro-fantasma")
)

func clientPath(mount, id string) string { return mount + "/data/clients/" + id }

// ReadClient devolve o registro e a versão do KV; (nil, 0, nil) se não existe.
func ReadClient(ctx context.Context, t Transport, mount, id string) (*ClientRecord, int, error) {
	r, err := t.Read(ctx, clientPath(mount, id))
	if err != nil {
		var ve *VaultError
		if errors.As(err, &ve) && ve.Status == 404 {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	d, _ := r["data"].(map[string]any)
	m, _ := d["data"].(map[string]any)
	if m == nil {
		return nil, 0, nil
	}
	meta, _ := d["metadata"].(map[string]any)
	version, _ := meta["version"].(float64)
	str := func(k string) string { s, _ := m[k].(string); return s }
	return &ClientRecord{Algo: str("algo"), Salt: str("salt"), Hash: str("hash"), Status: str("status"), CreatedAt: str("created_at")}, int(version), nil
}

func recordMap(r ClientRecord) map[string]any {
	m := map[string]any{"algo": r.Algo, "status": r.Status, "created_at": r.CreatedAt}
	if r.Salt != "" {
		m["salt"] = r.Salt
	}
	if r.Hash != "" {
		m["hash"] = r.Hash
	}
	return m
}

// WriteClient grava o registro com check-and-set: cas=0 cria (falha com ErrClientExists se já houver versão);
// cas=n substitui só se a versão atual for n (a rotação e a revogação não perdem escrita concorrente).
func WriteClient(ctx context.Context, t Transport, mount, id string, rec ClientRecord, cas int) error {
	_, err := t.Write(ctx, clientPath(mount, id), map[string]any{"data": recordMap(rec), "options": map[string]any{"cas": cas}})
	if err == nil {
		return nil
	}
	var ve *VaultError
	if errors.As(err, &ve) && ve.Status == 400 && strings.Contains(strings.ToLower(strings.Join(ve.Messages, " ")), "check-and-set") {
		if cas == 0 {
			return ErrClientExists
		}
		return fmt.Errorf("registro alterado por outra operação: %w", err)
	}
	return err
}

// VerifyClient confere o segredo do cliente em tempo constante. Segredo errado, cliente desconhecido e cliente
// revogado dão o mesmo false, depois do mesmo trabalho (uma leitura e um hash).
func (v *Vault) VerifyClient(ctx context.Context, id, secret string) (valid bool, err error) {
	t0 := time.Now()
	defer func() { v.observe("verify_client", t0, err) }()
	rec, _, err := ReadClient(ctx, v.t, v.cfg.MountClients, id)
	if err != nil {
		return false, translate(err)
	}
	salt, want, active := ghostSalt, ghostHash, false
	if rec != nil && rec.Status == ClientActive && rec.Algo == ClientAlgo {
		s, e1 := base64.StdEncoding.DecodeString(rec.Salt)
		h, e2 := base64.StdEncoding.DecodeString(rec.Hash)
		if e1 == nil && e2 == nil && len(h) == sha256.Size {
			salt, want, active = s, h, true
		}
	}
	match := subtle.ConstantTimeCompare(SecretHash(salt, secret), want) == 1
	return match && active, nil
}

var clientIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,63}$`)

// ValidClientID diz se o client_id tem o formato aceito (entra no caminho do Vault, então nunca tem "/" ou "..").
func ValidClientID(id string) bool { return clientIDPattern.MatchString(id) }

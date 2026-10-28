// Package provisionamento é a parte Vault do provisionamento de dongles: importa K_auth/K_rec no
// Transit (BYOK), roda o teste dourado, gera o material NVS e grava o registro.json que
// `tccelta provision register` (backend) usa para cadastrar o dongle no banco.
//
// Ordem das garantias: o KMS vem primeiro e o banco depois. No pior caso sobra uma chave órfã no
// Vault (inofensiva, sem uso); nunca um dongle no banco sem chave. Tudo é idempotente: chave que já
// existe no Transit é pulada e só o registro é reemitido.
package provisionamento

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tccelta/kms/internal/vault"
)

// SchemaRegistro é o schema do registro.json (docs/registro-dongle.md).
const SchemaRegistro = "tccelta.dongle.registro.v1"

// Registro é o contrato com o backend: o que o banco precisa saber de um dongle provisionado.
type Registro struct {
	Schema       string `json:"schema"`
	DevID        string `json:"dev_id"`
	Perfil       string `json:"perfil"`
	KMSChaveAuth string `json:"kms_chave_auth"`
	KMSChaveRec  string `json:"kms_chave_rec"`
	ChavePublica string `json:"chave_publica,omitempty"`
	Descricao    string `json:"descricao,omitempty"`
}

// Gravar escreve saida/<dev_id>/registro.json (0644: não tem segredo) e devolve o caminho.
func (r Registro) Gravar(saida string) (string, error) {
	dir := filepath.Join(saida, r.DevID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "registro.json")
	return p, os.WriteFile(p, append(b, '\n'), 0o600)
}

// Provisionador importa K_auth/K_rec no Vault.
type Provisionador struct {
	T         vault.Transporte
	MountAuth string
	MountRec  string
	Ambiente  string
	Log       *slog.Logger
}

func (p *Provisionador) chavePublica(ctx context.Context, mount string) (*rsa.PublicKey, string, error) {
	r, err := p.T.Ler(ctx, mount+"/wrapping_key")
	if err != nil {
		return nil, "", err
	}
	d, _ := r["data"].(map[string]any)
	s, _ := d["public_key"].(string)
	blk, _ := pem.Decode([]byte(s))
	if blk == nil {
		return nil, "", errors.New("wrapping_key sem PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, "", err
	}
	rk, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, "", errors.New("wrapping_key não é RSA")
	}
	der, _ := x509.MarshalPKIXPublicKey(rk)
	h := sha256.Sum256(der)
	return rk, hex.EncodeToString(h[:8]), nil
}

// ExportarWrapping grava kms_wrapping.json para o provision.py da Fase 1.
func (p *Provisionador) ExportarWrapping(ctx context.Context, saida string) error {
	out := map[string]any{"schema": "tccelta.kms.wrapping.v1"}
	for nome, m := range map[string]string{"auth": p.MountAuth, "rec": p.MountRec} {
		r, err := p.T.Ler(ctx, m+"/wrapping_key")
		if err != nil {
			return err
		}
		d, _ := r["data"].(map[string]any)
		_, kid, err := p.chavePublica(ctx, m)
		if err != nil {
			return err
		}
		out[nome] = map[string]any{"kid": kid, "pem": d["public_key"]}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return os.WriteFile(saida, append(b, '\n'), 0o600)
}

func (p *Provisionador) importarCiphertext(ctx context.Context, mount, nome, ct string) error {
	_, err := p.T.Escrever(ctx, fmt.Sprintf("%s/keys/%s/import", mount, nome), map[string]any{
		"ciphertext": ct, "hash_function": "SHA256", "type": "hmac",
		"exportable": false, "allow_plaintext_backup": false, "allow_rotation": false})
	return err
}

// existe diz se a chave `nome` já está no mount (o Vault responde 404 se não estiver).
func (p *Provisionador) existe(ctx context.Context, mount, nome string) (bool, error) {
	_, err := p.T.Ler(ctx, mount+"/keys/"+nome)
	if err == nil {
		return true, nil
	}
	var ev *vault.ErroVault
	if errors.As(err, &ev) && ev.Status == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

// importarSeFaltar importa o ciphertext só se a chave ainda não existir no mount.
func (p *Provisionador) importarSeFaltar(ctx context.Context, mount, nome string, ct func() (string, error)) error {
	ja, err := p.existe(ctx, mount, nome)
	if err != nil {
		return err
	}
	if ja {
		p.Log.Info("provisionamento.chave_ja_existe", "mount", mount, "chave", nome)
		return nil
	}
	c, err := ct()
	if err != nil {
		return err
	}
	if err := p.importarCiphertext(ctx, mount, nome, c); err != nil {
		p.Log.Error("provisionamento.import_falhou_chave_orfa_possivel", "mount", mount, "chave", nome, "erro", err)
		return fmt.Errorf("vault: %w", err)
	}
	return nil
}

// Dongle de teste/gerado.
type Dongle struct {
	DevID       []byte
	KAuth, KRec []byte
	Perfil      string
	KSig        []byte
}

// ImportarComChaves importa K_auth/K_rec do dongle (idempotente) e devolve o registro. As chaves do
// Transit levam o DevId em hexadecimal como nome.
func (p *Provisionador) ImportarComChaves(ctx context.Context, d Dongle, desc string) (Registro, error) {
	pa, _, err := p.chavePublica(ctx, p.MountAuth)
	if err != nil {
		return Registro{}, err
	}
	pr, _, err := p.chavePublica(ctx, p.MountRec)
	if err != nil {
		return Registro{}, err
	}
	hx := hex.EncodeToString(d.DevID)
	if err := p.importarSeFaltar(ctx, p.MountAuth, hx, func() (string, error) { return Embrulhar(pa, d.KAuth) }); err != nil {
		return Registro{}, err
	}
	if err := p.importarSeFaltar(ctx, p.MountRec, hx, func() (string, error) { return Embrulhar(pr, d.KRec) }); err != nil {
		return Registro{}, err
	}
	return Registro{Schema: SchemaRegistro, DevID: hx, Perfil: d.Perfil, KMSChaveAuth: hx, KMSChaveRec: hx, Descricao: desc}, nil
}

// Novo gera um dongle de teste, importa as chaves e devolve o dongle e o registro (recusado em producao).
// A checagem de DevId livre é só no Vault: todo dongle cadastrado tem chave lá.
func (p *Provisionador) Novo(ctx context.Context, perfil string) (Dongle, Registro, error) {
	if p.Ambiente == "producao" {
		return Dongle{}, Registro{}, errors.New("provisionar novo é recusado em AMBIENTE=producao")
	}
	for tent := 0; tent < 5; tent++ {
		d := Dongle{DevID: make([]byte, 8), KAuth: make([]byte, 32), KRec: make([]byte, 32), Perfil: perfil}
		for _, b := range [][]byte{d.DevID, d.KAuth, d.KRec} {
			if _, err := rand.Read(b); err != nil {
				return Dongle{}, Registro{}, err
			}
		}
		ja, err := p.existe(ctx, p.MountAuth, hex.EncodeToString(d.DevID))
		if err != nil {
			return Dongle{}, Registro{}, err
		}
		if ja {
			continue
		}
		r, err := p.ImportarComChaves(ctx, d, "dongle de teste")
		if err != nil {
			return Dongle{}, Registro{}, err
		}
		return d, r, nil
	}
	return Dongle{}, Registro{}, errors.New("não foi possível gerar um DevId livre")
}

// ImportarCSV importa dev_id_hex,k_auth_hex,k_rec_hex,perfil (só fora de produção). Idempotente.
func (p *Provisionador) ImportarCSV(ctx context.Context, arq string) ([]Dongle, []Registro, error) {
	if p.Ambiente == "producao" {
		return nil, nil, errors.New("importar chaves de teste é recusado em AMBIENTE=producao")
	}
	f, err := os.Open(arq)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rd.Comment = '#'
	rd.FieldsPerRecord = 4
	var ds []Dongle
	var rs []Registro
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		dev, e1 := hex.DecodeString(strings.TrimSpace(rec[0]))
		ka, e2 := hex.DecodeString(strings.TrimSpace(rec[1]))
		kr, e3 := hex.DecodeString(strings.TrimSpace(rec[2]))
		if e1 != nil || e2 != nil || e3 != nil || len(dev) != 8 || len(ka) != 32 || len(kr) != 32 {
			return nil, nil, fmt.Errorf("linha inválida: %v", rec[0])
		}
		d := Dongle{DevID: dev, KAuth: ka, KRec: kr, Perfil: strings.TrimSpace(rec[3])}
		r, err := p.ImportarComChaves(ctx, d, "dongle de teste (CSV)")
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", rec[0], err)
		}
		ds, rs = append(ds, d), append(rs, r)
	}
	return ds, rs, nil
}

// EnvelopeKMS é o envelope kms_record.json gerado pelo provision.py (Fase 1).
type EnvelopeKMS struct {
	Schema          string   `json:"schema"`
	DevID           string   `json:"dev_id"`
	SecurityProfile string   `json:"security_profile"`
	CreatedAt       string   `json:"created_at"`
	KAuth           chaveKMS `json:"k_auth"`
	KRec            chaveKMS `json:"k_rec"`
	KSigPublic      string   `json:"k_sig_public"`
}

type chaveKMS struct {
	Alg           string `json:"alg"`
	Kid           string `json:"kid"`
	CiphertextB64 string `json:"ciphertext_b64"`
	KeyHex        string `json:"key_hex"` // só alg "none" fora de produção
}

// ImportarRegistro importa o envelope de um dongle real (idempotente) e devolve o registro.
func (p *Provisionador) ImportarRegistro(ctx context.Context, arq, desc string) (Registro, error) {
	b, err := os.ReadFile(arq)
	if err != nil {
		return Registro{}, err
	}
	var r EnvelopeKMS
	if err := json.Unmarshal(b, &r); err != nil {
		return Registro{}, err
	}
	if r.Schema != "tccelta.kms.v1" {
		return Registro{}, fmt.Errorf("schema desconhecido %q", r.Schema)
	}
	dev, err := hex.DecodeString(r.DevID)
	if err != nil || len(dev) != 8 {
		return Registro{}, errors.New("dev_id inválido")
	}
	switch r.SecurityProfile {
	case "hmac", "asym", "none":
	default:
		return Registro{}, fmt.Errorf("security_profile %q", r.SecurityProfile)
	}
	pubSig := ""
	if r.SecurityProfile == "asym" {
		raw, err := hex.DecodeString(r.KSigPublic)
		if err != nil || len(raw) != 65 || raw[0] != 4 {
			return Registro{}, errors.New("k_sig_public inválida")
		}
		pubSig = r.KSigPublic
	}
	pa, kidA, err := p.chavePublica(ctx, p.MountAuth)
	if err != nil {
		return Registro{}, err
	}
	pr, kidR, err := p.chavePublica(ctx, p.MountRec)
	if err != nil {
		return Registro{}, err
	}
	ct := func(c chaveKMS, kid string, pub *rsa.PublicKey) func() (string, error) {
		return func() (string, error) {
			switch c.Alg {
			case "vault-byok-rsa-oaep-sha256-aes-kwp":
				if !strings.EqualFold(c.Kid, kid) {
					return "", fmt.Errorf("kid %s difere do da wrapping key atual %s", c.Kid, kid)
				}
				return c.CiphertextB64, nil
			case "none":
				if p.Ambiente == "producao" {
					return "", errors.New("alg none recusado em producao")
				}
				k, err := hex.DecodeString(c.KeyHex)
				if err != nil || len(k) != 32 {
					return "", errors.New("key_hex inválida")
				}
				return Embrulhar(pub, k)
			}
			return "", fmt.Errorf("alg %q", c.Alg)
		}
	}
	hx := hex.EncodeToString(dev)
	if err := p.importarSeFaltar(ctx, p.MountAuth, hx, ct(r.KAuth, kidA, pa)); err != nil {
		return Registro{}, err
	}
	if err := p.importarSeFaltar(ctx, p.MountRec, hx, ct(r.KRec, kidR, pr)); err != nil {
		return Registro{}, err
	}
	return Registro{Schema: SchemaRegistro, DevID: hx, Perfil: r.SecurityProfile, KMSChaveAuth: hx, KMSChaveRec: hx,
		ChavePublica: pubSig, Descricao: desc}, nil
}

// Verificar roda o teste dourado (1) e (2) com chaves conhecidas; exige a política de teste (dev).
func (p *Provisionador) Verificar(ctx context.Context, devHex string, kAuth, kRec []byte) error {
	entrada := []byte("teste-dourado")
	r, err := p.T.Escrever(ctx, fmt.Sprintf("%s/hmac/%s/sha2-256", p.MountAuth, devHex), map[string]any{"input": base64.StdEncoding.EncodeToString(entrada)})
	if err != nil {
		return fmt.Errorf("hmac no Vault: %w", err)
	}
	d, _ := r["data"].(map[string]any)
	s, _ := d["hmac"].(string)
	got, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "vault:v1:"))
	if err != nil {
		return err
	}
	m := hmac.New(sha256.New, kAuth)
	m.Write(entrada)
	if !hmac.Equal(m.Sum(nil), got) {
		return errors.New("TESTE DOURADO FALHOU: HMAC do Vault difere do HMAC local (tipo de chave/versão do Vault inadequados)")
	}
	m = hmac.New(sha256.New, kRec)
	m.Write(entrada)
	tag := m.Sum(nil)
	verif := func(t []byte) (bool, error) {
		r, err := p.T.Escrever(ctx, fmt.Sprintf("%s/verify/%s/sha2-256", p.MountRec, devHex),
			map[string]any{"input": base64.StdEncoding.EncodeToString(entrada), "hmac": "vault:v1:" + base64.StdEncoding.EncodeToString(t)})
		if err != nil {
			return false, err
		}
		d, _ := r["data"].(map[string]any)
		v, _ := d["valid"].(bool)
		return v, nil
	}
	if ok, err := verif(tag); err != nil || !ok {
		return fmt.Errorf("verify com tag correta deveria ser válido (%v)", err)
	}
	ruim := bytes.Clone(tag)
	ruim[0] ^= 1
	if ok, err := verif(ruim); err != nil || ok {
		return fmt.Errorf("verify com 1 bit trocado deveria ser inválido (%v)", err)
	}
	return nil
}

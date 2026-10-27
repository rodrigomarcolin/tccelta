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
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/kms"
)

// Provisionador cadastra dongles e importa K_auth/K_rec no Vault.
type Provisionador struct {
	T         kms.Transporte
	Pool      *pgxpool.Pool
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

func (p *Provisionador) importar(ctx context.Context, mount, nome string, pub *rsa.PublicKey, chave []byte) error {
	ct, err := Embrulhar(pub, chave)
	if err != nil {
		return err
	}
	return p.importarCiphertext(ctx, mount, nome, ct)
}

func (p *Provisionador) importarCiphertext(ctx context.Context, mount, nome, ct string) error {
	_, err := p.T.Escrever(ctx, fmt.Sprintf("%s/keys/%s/import", mount, nome), map[string]any{
		"ciphertext": ct, "hash_function": "SHA256", "type": "hmac",
		"exportable": false, "allow_plaintext_backup": false, "allow_rotation": false})
	return err
}

// cadastrar insere o dongle e executa importar dentro da mesma transação do banco.
func (p *Provisionador) cadastrar(ctx context.Context, dev []byte, perfil string, chavePub []byte, desc string, importar func(hex string) error) error {
	hexID := hex.EncodeToString(dev)
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := consultas.New(tx)
	var d *string
	if desc != "" {
		d = &desc
	}
	id, err := q.InserirDongle(ctx, consultas.InserirDongleParams{DevID: dev, PerfilSeguranca: perfil,
		KmsChaveAuth: hexID, KmsChaveRec: hexID, ChavePublica: chavePub, Descricao: d})
	if err != nil {
		return fmt.Errorf("cadastro: %w", err)
	}
	if err := q.InserirDongleEstado(ctx, id); err != nil {
		return err
	}
	if err := importar(hexID); err != nil {
		p.Log.Error("provisionamento.import_falhou_chave_orfa_possivel", "dev_id", hexID, "erro", err)
		return fmt.Errorf("vault: %w", err)
	}
	return tx.Commit(ctx)
}

// Dongle de teste/gerado.
type Dongle struct {
	DevID       []byte
	KAuth, KRec []byte
	Perfil      string
	KSig        []byte
}

// ImportarComChaves cadastra um dongle com chaves dadas (CSV de teste e `novo`).
func (p *Provisionador) ImportarComChaves(ctx context.Context, d Dongle, desc string) error {
	pa, _, err := p.chavePublica(ctx, p.MountAuth)
	if err != nil {
		return err
	}
	pr, _, err := p.chavePublica(ctx, p.MountRec)
	if err != nil {
		return err
	}
	return p.cadastrar(ctx, d.DevID, d.Perfil, nil, desc, func(h string) error {
		if err := p.importar(ctx, p.MountAuth, h, pa, d.KAuth); err != nil {
			return err
		}
		return p.importar(ctx, p.MountRec, h, pr, d.KRec)
	})
}

// Novo gera dongles de teste (recusado em producao).
func (p *Provisionador) Novo(ctx context.Context, perfil string) (Dongle, error) {
	if p.Ambiente == "producao" {
		return Dongle{}, errors.New("provisionar novo é recusado em AMBIENTE=producao")
	}
	for tent := 0; tent < 5; tent++ {
		d := Dongle{DevID: make([]byte, 8), KAuth: make([]byte, 32), KRec: make([]byte, 32), Perfil: perfil}
		for _, b := range [][]byte{d.DevID, d.KAuth, d.KRec} {
			if _, err := rand.Read(b); err != nil {
				return Dongle{}, err
			}
		}
		if _, err := consultas.New(p.Pool).BuscarDonglePorDevId(ctx, d.DevID); err == nil {
			continue
		}
		hx := hex.EncodeToString(d.DevID)
		if _, err := p.T.Ler(ctx, p.MountAuth+"/keys/"+hx); err == nil {
			continue
		}
		if err := p.ImportarComChaves(ctx, d, "dongle de teste"); err != nil {
			return Dongle{}, err
		}
		return d, nil
	}
	return Dongle{}, errors.New("não foi possível gerar um DevId livre")
}

// ImportarCSV importa dev_id_hex,k_auth_hex,k_rec_hex,perfil (só fora de produção). Idempotente.
func (p *Provisionador) ImportarCSV(ctx context.Context, arq string) ([]Dongle, error) {
	if p.Ambiente == "producao" {
		return nil, errors.New("importar chaves de teste é recusado em AMBIENTE=producao")
	}
	f, err := os.Open(arq)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rd.Comment = '#'
	rd.FieldsPerRecord = 4
	var out []Dongle
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		dev, e1 := hex.DecodeString(strings.TrimSpace(rec[0]))
		ka, e2 := hex.DecodeString(strings.TrimSpace(rec[1]))
		kr, e3 := hex.DecodeString(strings.TrimSpace(rec[2]))
		if e1 != nil || e2 != nil || e3 != nil || len(dev) != 8 || len(ka) != 32 || len(kr) != 32 {
			return nil, fmt.Errorf("linha inválida: %v", rec[0])
		}
		d := Dongle{DevID: dev, KAuth: ka, KRec: kr, Perfil: strings.TrimSpace(rec[3])}
		if _, err := consultas.New(p.Pool).BuscarDonglePorDevId(ctx, dev); err == nil {
			p.Log.Info("provisionamento.ja_existe", "dev_id", rec[0])
			out = append(out, d)
			continue
		}
		if err := p.ImportarComChaves(ctx, d, "dongle de teste (CSV)"); err != nil {
			// chave já importada no Vault (execução anterior interrompida)? apenas relata
			return nil, fmt.Errorf("%s: %w", rec[0], err)
		}
		out = append(out, d)
	}
	return out, nil
}

// RegistroKMS é o envelope kms_record.json gerado pelo provision.py (Fase 1).
type RegistroKMS struct {
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

// ImportarRegistro importa o envelope de um dongle real.
func (p *Provisionador) ImportarRegistro(ctx context.Context, arq, desc string) error {
	b, err := os.ReadFile(arq)
	if err != nil {
		return err
	}
	var r RegistroKMS
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	if r.Schema != "tccelta.kms.v1" {
		return fmt.Errorf("schema desconhecido %q", r.Schema)
	}
	dev, err := hex.DecodeString(r.DevID)
	if err != nil || len(dev) != 8 {
		return errors.New("dev_id inválido")
	}
	switch r.SecurityProfile {
	case "hmac", "asym", "none":
	default:
		return fmt.Errorf("security_profile %q", r.SecurityProfile)
	}
	var pubSig []byte
	if r.SecurityProfile == "asym" {
		pubSig, err = hex.DecodeString(r.KSigPublic)
		if err != nil || len(pubSig) != 65 || pubSig[0] != 4 {
			return errors.New("k_sig_public inválida")
		}
	}
	pa, kidA, err := p.chavePublica(ctx, p.MountAuth)
	if err != nil {
		return err
	}
	pr, kidR, err := p.chavePublica(ctx, p.MountRec)
	if err != nil {
		return err
	}
	ct := func(c chaveKMS, kid string, pub *rsa.PublicKey) (string, error) {
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
	ca, err := ct(r.KAuth, kidA, pa)
	if err != nil {
		return err
	}
	cr, err := ct(r.KRec, kidR, pr)
	if err != nil {
		return err
	}
	return p.cadastrar(ctx, dev, r.SecurityProfile, pubSig, desc, func(h string) error {
		if err := p.importarCiphertext(ctx, p.MountAuth, h, ca); err != nil {
			return err
		}
		return p.importarCiphertext(ctx, p.MountRec, h, cr)
	})
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

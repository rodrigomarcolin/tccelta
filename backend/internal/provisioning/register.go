// Package provisionamento cadastra no banco os dongles que o serviço KMS já provisionou. O contrato
// entre os dois lados é o arquivo registro.json (kms/docs/registro-dongle.md): a API e o KMS não
// compartilham código.
package provisionamento

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/banco/consultas"
)

// SchemaRegistro é o schema do registro.json.
const SchemaRegistro = "tccelta.dongle.registro.v1"

// Registro é o conteúdo de registro.json.
type Registro struct {
	Schema       string `json:"schema"`
	DevID        string `json:"dev_id"` // 16 dígitos hexadecimais
	Perfil       string `json:"perfil"` // hmac | asym | none
	KMSChaveAuth string `json:"kms_chave_auth"`
	KMSChaveRec  string `json:"kms_chave_rec"`
	ChavePublica string `json:"chave_publica,omitempty"` // hex, 65 B não comprimidos (só perfil asym)
	Descricao    string `json:"descricao,omitempty"`
}

// LerRegistro lê e valida um registro.json.
func LerRegistro(arquivo string) (Registro, error) {
	b, err := os.ReadFile(arquivo)
	if err != nil {
		return Registro{}, err
	}
	var r Registro
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Registro{}, fmt.Errorf("%s: %w", arquivo, err)
	}
	if err := r.Validar(); err != nil {
		return Registro{}, fmt.Errorf("%s: %w", arquivo, err)
	}
	return r, nil
}

// Validar confere o schema e os campos.
func (r Registro) Validar() error {
	if r.Schema != SchemaRegistro {
		return fmt.Errorf("schema desconhecido %q", r.Schema)
	}
	if d, err := hex.DecodeString(r.DevID); err != nil || len(d) != 8 {
		return errors.New("dev_id inválido (esperado 16 dígitos hexadecimais)")
	}
	switch r.Perfil {
	case "hmac", "asym", "none":
	default:
		return fmt.Errorf("perfil %q", r.Perfil)
	}
	if r.KMSChaveAuth == "" || r.KMSChaveRec == "" {
		return errors.New("kms_chave_auth e kms_chave_rec são obrigatórias")
	}
	if r.Perfil == "asym" {
		p, err := hex.DecodeString(r.ChavePublica)
		if err != nil || len(p) != 65 || p[0] != 4 {
			return errors.New("chave_publica inválida (perfil asym exige 65 B não comprimidos)")
		}
	}
	return nil
}

// Registrar insere dongle e dongle_estado numa transação. Idempotente: DevId já cadastrado é pulado
// (criado = false).
func Registrar(ctx context.Context, pool *pgxpool.Pool, r Registro, log *slog.Logger) (criado bool, err error) {
	if err := r.Validar(); err != nil {
		return false, err
	}
	dev, _ := hex.DecodeString(r.DevID)
	if _, err := consultas.New(pool).BuscarDonglePorDevId(ctx, dev); err == nil {
		log.Info("provisionamento.ja_cadastrado", "dev_id", r.DevID)
		return false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var pub []byte
	if r.ChavePublica != "" {
		pub, _ = hex.DecodeString(r.ChavePublica)
	}
	var desc *string
	if r.Descricao != "" {
		d := r.Descricao
		desc = &d
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	q := consultas.New(tx)
	id, err := q.InserirDongle(ctx, consultas.InserirDongleParams{DevID: dev, PerfilSeguranca: r.Perfil,
		KmsChaveAuth: r.KMSChaveAuth, KmsChaveRec: r.KMSChaveRec, ChavePublica: pub, Descricao: desc})
	if err != nil {
		return false, fmt.Errorf("cadastro: %w", err)
	}
	if err := q.InserirDongleEstado(ctx, id); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	log.Info("provisionamento.cadastrado", "dev_id", r.DevID, "perfil", r.Perfil)
	return true, nil
}

// ArquivosRegistro lista os registro.json de um arquivo ou, se for diretório, de seus subdiretórios.
func ArquivosRegistro(caminho string) ([]string, error) {
	st, err := os.Stat(caminho)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return []string{caminho}, nil
	}
	var out []string
	err = filepath.WalkDir(caminho, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "registro.json" {
			out = append(out, p)
		}
		return err
	})
	sort.Strings(out)
	return out, err
}

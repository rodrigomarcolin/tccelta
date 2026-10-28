// Package provisioning cadastra no banco os dongles que o serviço KMS já provisionou. O contrato
// entre os dois lados é o arquivo record.json (docs/kms-dongle-record.md): a API e o KMS não
// compartilham código.
package provisioning

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

	"tccelta/backend/internal/database/dbgen"
)

// RecordSchema é o schema do record.json.
const RecordSchema = "tccelta.dongle.record.v1"

// Record é o conteúdo de record.json.
type Record struct {
	Schema      string `json:"schema"`
	DevID       string `json:"dev_id"`  // 16 dígitos hexadecimais
	Profile     string `json:"profile"` // hmac | asym | none
	KMSAuthKey  string `json:"kms_auth_key"`
	KMSRecKey   string `json:"kms_rec_key"`
	PublicKey   string `json:"public_key,omitempty"` // hex, 65 B não comprimidos (só perfil asym)
	Description string `json:"description,omitempty"`
}

// ReadRecord lê e valida um record.json.
func ReadRecord(file string) (Record, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return Record{}, err
	}
	var r Record
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Record{}, fmt.Errorf("%s: %w", file, err)
	}
	if err := r.Validate(); err != nil {
		return Record{}, fmt.Errorf("%s: %w", file, err)
	}
	return r, nil
}

// Validate confere o schema e os campos.
func (r Record) Validate() error {
	if r.Schema != RecordSchema {
		return fmt.Errorf("schema desconhecido %q", r.Schema)
	}
	if d, err := hex.DecodeString(r.DevID); err != nil || len(d) != 8 {
		return errors.New("dev_id inválido (esperado 16 dígitos hexadecimais)")
	}
	switch r.Profile {
	case "hmac", "asym", "none":
	default:
		return fmt.Errorf("perfil %q", r.Profile)
	}
	if r.KMSAuthKey == "" || r.KMSRecKey == "" {
		return errors.New("kms_auth_key e kms_rec_key são obrigatórias")
	}
	if r.Profile == "asym" {
		p, err := hex.DecodeString(r.PublicKey)
		if err != nil || len(p) != 65 || p[0] != 4 {
			return errors.New("public_key inválida (perfil asym exige 65 B não comprimidos)")
		}
	}
	return nil
}

// Register insere dongle e dongle_state numa transação. Idempotente: DevId já cadastrado é pulado
// (criado = false).
func Register(ctx context.Context, pool *pgxpool.Pool, r Record, log *slog.Logger) (created bool, err error) {
	if err := r.Validate(); err != nil {
		return false, err
	}
	dev, _ := hex.DecodeString(r.DevID)
	if _, err := dbgen.New(pool).FindDongleByDevID(ctx, dev); err == nil {
		log.Info("provisionamento.ja_cadastrado", "dev_id", r.DevID)
		return false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var pub []byte
	if r.PublicKey != "" {
		pub, _ = hex.DecodeString(r.PublicKey)
	}
	var desc *string
	if r.Description != "" {
		d := r.Description
		desc = &d
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	q := dbgen.New(tx)
	id, err := q.InsertDongle(ctx, dbgen.InsertDongleParams{DevID: dev, SecurityProfile: r.Profile,
		KmsAuthKey: r.KMSAuthKey, KmsRecKey: r.KMSRecKey, PublicKey: pub, Description: desc})
	if err != nil {
		return false, fmt.Errorf("cadastro: %w", err)
	}
	if err := q.InsertDongleState(ctx, id); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	log.Info("provisionamento.cadastrado", "dev_id", r.DevID, "perfil", r.Profile)
	return true, nil
}

// RecordFiles lista os record.json de um arquivo ou, se for diretório, de seus subdiretórios.
func RecordFiles(path string) ([]string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return []string{path}, nil
	}
	var out []string
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "record.json" {
			out = append(out, p)
		}
		return err
	})
	sort.Strings(out)
	return out, err
}

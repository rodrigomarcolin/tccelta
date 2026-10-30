// Package provisioning lê e valida o record.json dos dongles que o serviço KMS já provisionou; o cadastro no banco é o
// caso de uso usecase/provisioning. O contrato entre os dois lados é o arquivo
// record.json (docs/kms-dongle-record.md): a API e o KMS não compartilham código.
package provisioning

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tccelta/backend/internal/domain/dongle"
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

// Registration valida o record e o converte no cadastro que o caso de uso usecase/provisioning grava.
func (r Record) Registration() (dongle.Registration, error) {
	if err := r.Validate(); err != nil {
		return dongle.Registration{}, err
	}
	var g dongle.Registration
	dev, _ := hex.DecodeString(r.DevID)
	copy(g.DevID[:], dev)
	g.Profile, g.AuthKey, g.RecKey, g.Description = r.Profile, r.KMSAuthKey, r.KMSRecKey, r.Description
	if r.PublicKey != "" {
		g.PublicKey, _ = hex.DecodeString(r.PublicKey)
	}
	return g, nil
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

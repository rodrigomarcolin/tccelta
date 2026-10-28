package main

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/tools/provisioning"
	"tccelta/backend/internal/tools/simulator"
)

// cmdProvision cadastra no banco os dongles que o serviço KMS provisionou (`tccelta-kms provisionar`).
// A parte do Vault não existe mais aqui: o contrato entre as duas pontas é o arquivo record.json.
func cmdProvision(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "register" {
		return fmt.Errorf("uso: provision register --file record.json|diretório")
	}
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	file := fs.String("file", "", "record.json, ou diretório com subpastas <devid>/record.json")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("informe --file")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	files, err := provisioning.RecordFiles(*file)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("nenhum record.json em %s", *file)
	}
	regs := make([]provisioning.Record, 0, len(files))
	for _, a := range files { // valida tudo antes de tocar no banco
		r, err := provisioning.ReadRecord(a)
		if err != nil {
			return err
		}
		regs = append(regs, r)
	}
	pool, err := poolFromEnv(ctx, "", log)
	if err != nil {
		return err
	}
	defer pool.Close()
	for _, r := range regs {
		created, err := provisioning.Register(ctx, pool, r, log)
		if err != nil {
			return fmt.Errorf("dongle %s: %w", r.DevID, err)
		}
		if created {
			fmt.Printf("dongle %s cadastrado\n", r.DevID)
		} else {
			fmt.Printf("dongle %s já estava cadastrado\n", r.DevID)
		}
	}
	return nil
}

// readKeysCSV lê dev_id_hex,k_auth_hex,k_rec_hex,perfil (dongles de teste do simulador).
func readKeysCSV(file string) ([]simulator.Keys, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rd.Comment = '#'
	rd.FieldsPerRecord = 4
	recs, err := rd.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []simulator.Keys
	for _, r := range recs {
		var c simulator.Keys
		dev, e1 := hex.DecodeString(strings.TrimSpace(r[0]))
		ka, e2 := hex.DecodeString(strings.TrimSpace(r[1]))
		kr, e3 := hex.DecodeString(strings.TrimSpace(r[2]))
		if e1 != nil || e2 != nil || e3 != nil || len(dev) != 8 {
			return nil, fmt.Errorf("linha inválida %q", r[0])
		}
		copy(c.Dev[:], dev)
		c.KAuth, c.KRec = ka, kr
		out = append(out, c)
	}
	return out, nil
}

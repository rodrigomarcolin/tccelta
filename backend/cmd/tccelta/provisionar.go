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
	"tccelta/backend/internal/provisionamento"
	"tccelta/backend/internal/simulador"
)

// cmdProvisionar cadastra no banco os dongles que o serviço KMS provisionou (`tccelta-kms provisionar`).
// A parte do Vault não existe mais aqui: o contrato entre as duas pontas é o arquivo registro.json.
func cmdProvisionar(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "registrar" {
		return fmt.Errorf("uso: provisionar registrar --arquivo registro.json|diretório")
	}
	fs := flag.NewFlagSet("registrar", flag.ContinueOnError)
	arq := fs.String("arquivo", "", "registro.json, ou diretório com subpastas <devid>/registro.json")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *arq == "" {
		return fmt.Errorf("informe --arquivo")
	}
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	log := novoLog(cfg)
	arquivos, err := provisionamento.ArquivosRegistro(*arq)
	if err != nil {
		return err
	}
	if len(arquivos) == 0 {
		return fmt.Errorf("nenhum registro.json em %s", *arq)
	}
	regs := make([]provisionamento.Registro, 0, len(arquivos))
	for _, a := range arquivos { // valida tudo antes de tocar no banco
		r, err := provisionamento.LerRegistro(a)
		if err != nil {
			return err
		}
		regs = append(regs, r)
	}
	pool, err := poolDeEnv(ctx, "", log)
	if err != nil {
		return err
	}
	defer pool.Close()
	for _, r := range regs {
		criado, err := provisionamento.Registrar(ctx, pool, r, log)
		if err != nil {
			return fmt.Errorf("dongle %s: %w", r.DevID, err)
		}
		if criado {
			fmt.Printf("dongle %s cadastrado\n", r.DevID)
		} else {
			fmt.Printf("dongle %s já estava cadastrado\n", r.DevID)
		}
	}
	return nil
}

// lerCSVChaves lê dev_id_hex,k_auth_hex,k_rec_hex,perfil (dongles de teste do simulador).
func lerCSVChaves(arq string) ([]simulador.Chaves, error) {
	f, err := os.Open(arq)
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
	var out []simulador.Chaves
	for _, r := range recs {
		var c simulador.Chaves
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

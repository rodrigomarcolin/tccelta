package main

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/kms"
	"tccelta/backend/internal/provisionamento"
	"tccelta/backend/internal/simulador"
)

func novoProvisionador(ctx context.Context) (*provisionamento.Provisionador, func(), error) {
	cfg, err := config.Carregar()
	if err != nil {
		return nil, nil, err
	}
	log := novoLog(cfg)
	pool, err := poolDeEnv(ctx, "", log)
	if err != nil {
		return nil, nil, err
	}
	cli, err := kms.NovoCliente(ctx, cfg.Vault, log)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	p := &provisionamento.Provisionador{T: cli, Pool: pool, MountAuth: cfg.Vault.MountAuth, MountRec: cfg.Vault.MountRec,
		Ambiente: cfg.Ambiente, Log: log}
	return p, func() { cli.Fechar(); pool.Close() }, nil
}

func cmdProvisionar(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: provisionar <novo|importar|importar-registro|exportar-wrapping|verificar>")
	}
	p, fechar, err := novoProvisionador(ctx)
	if err != nil {
		return err
	}
	defer fechar()
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	saida := fs.String("saida", "/saida", "diretório (ou arquivo, em exportar-wrapping) de saída")
	qtd := fs.Int("quantidade", 1, "quantidade de dongles de teste")
	perfil := fs.String("perfil", "hmac", "hmac|asym|none")
	hw := fs.String("hw", "dev", "dev|devsec")
	desc := fs.String("descricao", "", "descrição")
	arq := fs.String("arquivo", "", "arquivo de entrada")
	devID := fs.String("dev-id", "", "DevId (verificar)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "exportar-wrapping":
		return p.ExportarWrapping(ctx, *saida)
	case "importar-registro":
		if err := p.ImportarRegistro(ctx, *arq, *desc); err != nil {
			return err
		}
		fmt.Println("registro importado")
		return nil
	case "novo":
		for i := 0; i < *qtd; i++ {
			d, err := p.Novo(ctx, *perfil)
			if err != nil {
				return err
			}
			dir, err := provisionamento.GerarMaterial(ctx, d, *saida, *hw)
			if err != nil {
				return err
			}
			fmt.Printf("dongle %s provisionado em %s (apague bench_keys.json depois de usar)\n", hex.EncodeToString(d.DevID), dir)
		}
		return nil
	case "importar":
		ds, err := p.ImportarCSV(ctx, *arq)
		if err != nil {
			return err
		}
		for _, d := range ds {
			hx := hex.EncodeToString(d.DevID)
			if err := p.Verificar(ctx, hx, d.KAuth, d.KRec); err != nil {
				return fmt.Errorf("dongle %s: %w", hx, err)
			}
			fmt.Printf("dongle %s importado; teste dourado OK\n", hx)
		}
		return nil
	case "verificar":
		ds, err := lerCSVChaves(*arq)
		if err != nil {
			return err
		}
		for _, d := range ds {
			if *devID != "" && !strings.EqualFold(*devID, d.Dev.Hex()) {
				continue
			}
			if err := p.Verificar(ctx, d.Dev.Hex(), d.KAuth, d.KRec); err != nil {
				return err
			}
			fmt.Printf("dongle %s: teste dourado OK\n", d.Dev.Hex())
		}
		return nil
	}
	return fmt.Errorf("subcomando desconhecido %q", args[0])
}

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

func cmdJWT(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: jwt <rotacionar [--agora]|agendar>")
	}
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	log := novoLog(cfg)
	cli, err := kms.NovoCliente(ctx, cfg.Vault, log)
	if err != nil {
		return err
	}
	defer cli.Fechar()
	rot := &kms.Rotacionador{T: cli, Mount: cfg.Vault.MountJWT, Chave: cfg.Vault.ChaveJWT, Periodo: cfg.Rotacao.Periodo,
		Validade: cfg.JWT.Validade + cfg.JWT.Folga + 10*time.Minute, Log: log}
	switch args[0] {
	case "rotacionar":
		forcar := len(args) > 1 && args[1] == "--agora"
		rotacionou, min, err := rot.Verificar(ctx, forcar)
		fmt.Printf("rotacionou=%v min_decryption_version=%d\n", rotacionou, min)
		return err
	case "agendar":
		go servirSaude(ctx, ":8092")
		rot.Agendar(ctx, cfg.Rotacao.Verificacao)
		return nil
	}
	return fmt.Errorf("subcomando desconhecido %q", args[0])
}

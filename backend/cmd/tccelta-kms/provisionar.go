package main

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/provisionamento"
	"tccelta/backend/internal/kms/vault"
)

// cmdProvisionar importa as chaves no Vault e grava <saida>/<devid>/registro.json, que o backend
// cadastra no banco com `tccelta provision register`.
func cmdProvisionar(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: provisionar <novo|importar|importar-registro|exportar-wrapping|verificar>")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	saida := fs.String("saida", "/saida", "diretório de saída dos registros (ou arquivo, em exportar-wrapping)")
	qtd := fs.Int("quantidade", 1, "quantidade de dongles de teste (novo)")
	perfil := fs.String("perfil", "hmac", "hmac|asym|none (novo)")
	hw := fs.String("hw", "dev", "dev|devsec (novo)")
	desc := fs.String("descricao", "", "descrição")
	arq := fs.String("arquivo", "", "arquivo de entrada")
	devID := fs.String("dev-id", "", "DevId (verificar)")
	semMaterial := fs.Bool("sem-material", false, "novo: so importa as chaves e grava o registro.json (sem gerar a imagem NVS)")
	csvSaida := fs.String("csv-saida", "", "acrescenta dev_id,k_auth,k_rec,perfil dos dongles criados (novo), para o simulador")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	log := novoLog(cfg)
	cli, err := vault.NovoCliente(ctx, cfg.Vault, log)
	if err != nil {
		return err
	}
	defer cli.Fechar()
	p := &provisionamento.Provisionador{T: cli, MountAuth: cfg.Vault.MountAuth, MountRec: cfg.Vault.MountRec,
		Ambiente: cfg.Ambiente, Log: log}

	grava := func(r provisionamento.Registro) error {
		caminho, err := r.Gravar(*saida)
		if err != nil {
			return err
		}
		fmt.Printf("dongle %s: registro em %s\n", r.DevID, caminho)
		return nil
	}

	switch args[0] {
	case "exportar-wrapping":
		return p.ExportarWrapping(ctx, *saida)
	case "importar-registro":
		r, err := p.ImportarRegistro(ctx, *arq, *desc)
		if err != nil {
			return err
		}
		return grava(r)
	case "novo":
		for i := 0; i < *qtd; i++ {
			d, r, err := p.Novo(ctx, *perfil)
			if err != nil {
				return err
			}
			if !*semMaterial {
				dir, err := provisionamento.GerarMaterial(ctx, d, *saida, *hw)
				if err != nil {
					return err
				}
				fmt.Printf("dongle %s provisionado em %s (apague bench_keys.json depois de usar)\n", hex.EncodeToString(d.DevID), dir)
			}
			if err := grava(r); err != nil {
				return err
			}
			if *csvSaida != "" {
				if err := acrescentarCSV(*csvSaida, d); err != nil {
					return err
				}
			}
		}
		return nil
	case "importar":
		ds, rs, err := p.ImportarCSV(ctx, *arq)
		if err != nil {
			return err
		}
		for i, d := range ds {
			hx := hex.EncodeToString(d.DevID)
			if err := p.Verificar(ctx, hx, d.KAuth, d.KRec); err != nil {
				return fmt.Errorf("dongle %s: %w", hx, err)
			}
			fmt.Printf("dongle %s importado; teste dourado OK\n", hx)
			if err := grava(rs[i]); err != nil {
				return err
			}
		}
		return nil
	case "verificar":
		ds, err := lerCSVChaves(*arq)
		if err != nil {
			return err
		}
		for _, d := range ds {
			hx := hex.EncodeToString(d.dev)
			if *devID != "" && !strings.EqualFold(*devID, hx) {
				continue
			}
			if err := p.Verificar(ctx, hx, d.kAuth, d.kRec); err != nil {
				return err
			}
			fmt.Printf("dongle %s: teste dourado OK\n", hx)
		}
		return nil
	}
	return fmt.Errorf("subcomando desconhecido %q", args[0])
}

type chavesCSV struct{ dev, kAuth, kRec []byte }

func lerCSVChaves(arq string) ([]chavesCSV, error) {
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
	var out []chavesCSV
	for _, r := range recs {
		dev, e1 := hex.DecodeString(strings.TrimSpace(r[0]))
		ka, e2 := hex.DecodeString(strings.TrimSpace(r[1]))
		kr, e3 := hex.DecodeString(strings.TrimSpace(r[2]))
		if e1 != nil || e2 != nil || e3 != nil || len(dev) != 8 {
			return nil, fmt.Errorf("linha inválida %q", r[0])
		}
		out = append(out, chavesCSV{dev, ka, kr})
	}
	return out, nil
}

// acrescentarCSV grava as chaves do dongle de teste no CSV do simulador (0600: tem segredo).
func acrescentarCSV(arq string, d provisionamento.Dongle) error {
	f, err := os.OpenFile(arq, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s,%s,%s,%s\n", hex.EncodeToString(d.DevID), hex.EncodeToString(d.KAuth), hex.EncodeToString(d.KRec), d.Perfil)
	return err
}

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
	"tccelta/backend/internal/kms/provisioning"
	"tccelta/backend/internal/kms/vault"
)

// cmdProvision importa as chaves no Vault e grava <out>/<devid>/record.json, que o backend
// cadastra no banco com `tccelta provision register`.
func cmdProvision(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: provision <new|import|import-record|export-wrapping|verify>")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	outDir := fs.String("out", "/saida", "diretório de saída dos registros (ou arquivo, em export-wrapping)")
	count := fs.Int("count", 1, "quantidade de dongles de teste (new)")
	profile := fs.String("profile", "hmac", "hmac|asym|none (new)")
	hw := fs.String("hw", "dev", "dev|devsec (new)")
	desc := fs.String("description", "", "descrição")
	file := fs.String("file", "", "arquivo de entrada")
	devID := fs.String("dev-id", "", "DevId (verify)")
	noMaterial := fs.Bool("no-material", false, "new: so importa as chaves e grava o record.json (sem gerar a imagem NVS)")
	csvOut := fs.String("csv-out", "", "acrescenta dev_id,k_auth,k_rec,profile dos dongles criados (new), para o simulador")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLog(cfg)
	cli, err := vault.NewClient(ctx, cfg.Vault, log)
	if err != nil {
		return err
	}
	defer cli.Close()
	p := &provisioning.Provisioner{T: cli, MountAuth: cfg.Vault.MountAuth, MountRec: cfg.Vault.MountRec,
		Environment: cfg.Environment, Log: log}

	save := func(r provisioning.Record) error {
		path, err := r.Save(*outDir)
		if err != nil {
			return err
		}
		fmt.Printf("dongle %s: registro em %s\n", r.DevID, path)
		return nil
	}

	switch args[0] {
	case "export-wrapping":
		return p.ExportWrapping(ctx, *outDir)
	case "import-record":
		r, err := p.ImportRecord(ctx, *file, *desc)
		if err != nil {
			return err
		}
		return save(r)
	case "new":
		for i := 0; i < *count; i++ {
			d, r, err := p.NewDongle(ctx, *profile)
			if err != nil {
				return err
			}
			if !*noMaterial {
				dir, err := provisioning.GenerateMaterial(ctx, d, *outDir, *hw)
				if err != nil {
					return err
				}
				fmt.Printf("dongle %s provisionado em %s (apague bench_keys.json depois de usar)\n", hex.EncodeToString(d.DevID), dir)
			}
			if err := save(r); err != nil {
				return err
			}
			if *csvOut != "" {
				if err := appendCSV(*csvOut, d); err != nil {
					return err
				}
			}
		}
		return nil
	case "import":
		ds, rs, err := p.ImportCSV(ctx, *file)
		if err != nil {
			return err
		}
		for i, d := range ds {
			hx := hex.EncodeToString(d.DevID)
			if err := p.VerifyGolden(ctx, hx, d.KAuth, d.KRec); err != nil {
				return fmt.Errorf("dongle %s: %w", hx, err)
			}
			fmt.Printf("dongle %s importado; teste dourado OK\n", hx)
			if err := save(rs[i]); err != nil {
				return err
			}
		}
		return nil
	case "verify":
		ds, err := readKeysCSV(*file)
		if err != nil {
			return err
		}
		for _, d := range ds {
			hx := hex.EncodeToString(d.dev)
			if *devID != "" && !strings.EqualFold(*devID, hx) {
				continue
			}
			if err := p.VerifyGolden(ctx, hx, d.kAuth, d.kRec); err != nil {
				return err
			}
			fmt.Printf("dongle %s: teste dourado OK\n", hx)
		}
		return nil
	}
	return fmt.Errorf("subcomando desconhecido %q", args[0])
}

type keyRecord struct{ dev, kAuth, kRec []byte }

func readKeysCSV(file string) ([]keyRecord, error) {
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
	var out []keyRecord
	for _, r := range recs {
		dev, e1 := hex.DecodeString(strings.TrimSpace(r[0]))
		ka, e2 := hex.DecodeString(strings.TrimSpace(r[1]))
		kr, e3 := hex.DecodeString(strings.TrimSpace(r[2]))
		if e1 != nil || e2 != nil || e3 != nil || len(dev) != 8 {
			return nil, fmt.Errorf("linha inválida %q", r[0])
		}
		out = append(out, keyRecord{dev, ka, kr})
	}
	return out, nil
}

// appendCSV grava as chaves do dongle de teste no CSV do simulador (0600: tem segredo).
func appendCSV(file string, d provisioning.Dongle) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s,%s,%s,%s\n", hex.EncodeToString(d.DevID), hex.EncodeToString(d.KAuth), hex.EncodeToString(d.KRec), d.Profile)
	return err
}

package main

import (
	"context"
	"flag"
	"fmt"

	"tccelta/backend/internal/adapter/postgres"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/tools/provisioning"
	provisioningUC "tccelta/backend/internal/usecase/provisioning"
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
	regs := make([]dongle.Registration, 0, len(files))
	for _, a := range files { // valida tudo antes de tocar no banco
		r, err := provisioning.ReadRecord(a)
		if err != nil {
			return err
		}
		g, err := r.Registration()
		if err != nil {
			return fmt.Errorf("%s: %w", a, err)
		}
		regs = append(regs, g)
	}
	pool, err := poolFromEnv(ctx, "", log)
	if err != nil {
		return err
	}
	defer pool.Close()
	registrar := provisioningUC.New(postgres.Repositories(dbgen.New(pool)), postgres.NewTransactor(pool), log)
	for _, r := range regs {
		created, err := registrar.Register(ctx, r)
		if err != nil {
			return fmt.Errorf("dongle %s: %w", r.DevID.Hex(), err)
		}
		if created {
			fmt.Printf("dongle %s cadastrado\n", r.DevID)
		} else {
			fmt.Printf("dongle %s já estava cadastrado\n", r.DevID)
		}
	}
	return nil
}

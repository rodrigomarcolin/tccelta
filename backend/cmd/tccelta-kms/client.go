package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/provisioning"
	"tccelta/backend/internal/kms/vault"
)

// cmdClient gerencia os clientes de client credentials (create, rotate, revoke) com o token do provisionador.
// O segredo sai uma única vez: em stdout ou, com --out, num arquivo 0400.
func cmdClient(ctx context.Context, args []string) error {
	const usage = "uso: client <create|rotate|revoke> --id ID [--out ARQUIVO]"
	if len(args) == 0 {
		return errors.New(usage)
	}
	fs := flag.NewFlagSet("client "+args[0], flag.ContinueOnError)
	id := fs.String("id", "", "client_id")
	out := fs.String("out", "", "grava o segredo neste arquivo (modo 0400) em vez de imprimir")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *id == "" {
		return errors.New(usage)
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
	c := &provisioning.Clients{T: cli, Mount: cfg.Vault.MountClients, Log: log}

	var secret string
	switch args[0] {
	case "create":
		secret, err = c.Create(ctx, *id)
	case "rotate":
		secret, err = c.Rotate(ctx, *id)
	case "revoke":
		if err := c.Revoke(ctx, *id); err != nil {
			return err
		}
		fmt.Printf("cliente %s revogado\n", *id)
		return nil
	default:
		return fmt.Errorf("subcomando desconhecido %q", args[0])
	}
	if err != nil {
		return err
	}
	if *out != "" {
		if err := provisioning.WriteSecretFile(*out, secret); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "cliente %s: segredo gravado em %s\n", *id, *out)
		return nil
	}
	fmt.Fprintf(os.Stderr, "cliente %s: segredo abaixo (mostrado uma única vez)\n", *id)
	fmt.Println(secret)
	return nil
}

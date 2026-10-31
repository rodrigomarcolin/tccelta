package provisioning

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const nvsSize = "0x3000"

// CSVSecnvs monta o CSV da partição secnvs (namespace sec) só na memória.
func CSVSecnvs(d Dongle) []byte {
	prof := map[string]int{"none": 0, "hmac": 1, "asym": 2}[d.Profile]
	var b bytes.Buffer
	b.WriteString("key,type,encoding,value\nsec,namespace,,\nprov_ver,data,u8,1\n")
	fmt.Fprintf(&b, "dev_id,data,hex2bin,%s\n", hex.EncodeToString(d.DevID))
	fmt.Fprintf(&b, "k_auth,data,hex2bin,%s\n", hex.EncodeToString(d.KAuth))
	fmt.Fprintf(&b, "k_rec,data,hex2bin,%s\n", hex.EncodeToString(d.KRec))
	fmt.Fprintf(&b, "prof,data,u8,%d\n", prof)
	if d.Profile == "asym" && len(d.KSig) == 32 {
		fmt.Fprintf(&b, "k_sig,data,hex2bin,%s\n", hex.EncodeToString(d.KSig))
	}
	return b.Bytes()
}

// diretório em RAM para o CSV efêmero (o gerador não aceita stdin de forma confiável).
func ramDir() string {
	if st, err := os.Stat("/dev/shm"); err == nil && st.IsDir() {
		return "/dev/shm"
	}
	return os.TempDir()
}

// GenerateMaterial grava outDir/<hex>/ com secnvs.bin (imagem NVS), bench_keys.json e dongle.json.
// hw: "dev" (imagem em claro) ou "devsec" (cifrada, grava nvs_keys.bin).
func GenerateMaterial(ctx context.Context, d Dongle, outDir, hw string) (string, error) {
	hx := hex.EncodeToString(d.DevID)
	dir := filepath.Join(outDir, hx)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	csv, err := os.CreateTemp(ramDir(), "secnvs-*.csv")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(csv.Name()) }()
	_ = csv.Chmod(0o600)
	content := CSVSecnvs(d)
	_, err = csv.Write(content)
	zero(content)
	if cerr := csv.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "secnvs.bin")
	run := func(args ...string) error {
		c := exec.CommandContext(ctx, "python", append([]string{"-m", "esp_idf_nvs_partition_gen"}, args...)...) //nolint:gosec // args são caminhos internos e literais, sem entrada externa
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("esp_idf_nvs_partition_gen %v: %w: %s", args[0], err, out)
		}
		return nil
	}
	switch hw {
	case "devsec":
		keys := filepath.Join(dir, "nvs_keys.bin")
		if err := run("generate-key", "--keyfile", keys); err != nil {
			return "", err
		}
		_ = os.Chmod(keys, 0o600)
		if err := run("encrypt", csv.Name(), bin, nvsSize, "--inputkey", keys); err != nil {
			return "", err
		}
	default:
		if err := run("generate", csv.Name(), bin, nvsSize); err != nil {
			return "", err
		}
	}
	_ = os.Chmod(bin, 0o600)
	benchKeys := map[string]string{"dev_id": hx, "k_auth": hex.EncodeToString(d.KAuth), "k_rec": hex.EncodeToString(d.KRec)}
	if d.Profile == "asym" {
		pub, err := d.PublicKey()
		if err != nil {
			return "", err
		}
		benchKeys["k_sig"] = hex.EncodeToString(d.KSig)
		benchKeys["k_sig_public"] = hex.EncodeToString(pub)
	}
	bench, _ := json.MarshalIndent(benchKeys, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "bench_keys.json"), bench, 0o600); err != nil {
		return "", err
	}
	info, _ := json.MarshalIndent(map[string]any{"dev_id": hx, "profile": d.Profile, "created_at": time.Now().UTC().Format(time.RFC3339),
		"kms": map[string]string{"auth": "transit-auth/" + hx, "rec": "transit-rec/" + hx}}, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "dongle.json"), info, 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

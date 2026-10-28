package provisionamento

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tccelta/kms/internal/vault"
)

// fakeVault guarda quais chaves existem e quantas importações aconteceram.
type fakeVault struct {
	pub       string
	chaves    map[string]bool // "<mount>/<nome>"
	importado []string
}

func novoFake(t *testing.T) *fakeVault {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	return &fakeVault{pub: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), chaves: map[string]bool{}}
}

func (f *fakeVault) Ler(_ context.Context, caminho string) (map[string]any, error) {
	if strings.HasSuffix(caminho, "/wrapping_key") {
		return map[string]any{"data": map[string]any{"public_key": f.pub}}, nil
	}
	if p := strings.SplitN(caminho, "/keys/", 2); len(p) == 2 && f.chaves[p[0]+"/"+p[1]] {
		return map[string]any{"data": map[string]any{"type": "hmac"}}, nil
	}
	return nil, &vault.ErroVault{Status: 404, Msgs: []string{"não encontrada"}}
}

func (f *fakeVault) Escrever(_ context.Context, caminho string, corpo map[string]any) (map[string]any, error) {
	p := strings.Split(caminho, "/") // <mount>/keys/<nome>/import
	if len(p) != 4 || p[1] != "keys" || p[3] != "import" || corpo["ciphertext"] == "" || corpo["exportable"] != false {
		return nil, &vault.ErroVault{Status: 400, Msgs: []string{"requisição inesperada " + caminho}}
	}
	f.chaves[p[0]+"/"+p[2]] = true
	f.importado = append(f.importado, p[0]+"/"+p[2])
	return map[string]any{}, nil
}

func novoProv(f *fakeVault, ambiente string) *Provisionador {
	return &Provisionador{T: f, MountAuth: "transit-auth", MountRec: "transit-rec", Ambiente: ambiente,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func dongle(t *testing.T) Dongle {
	t.Helper()
	d := Dongle{DevID: make([]byte, 8), KAuth: make([]byte, 32), KRec: make([]byte, 32), Perfil: "hmac"}
	for _, b := range [][]byte{d.DevID, d.KAuth, d.KRec} {
		_, _ = rand.Read(b)
	}
	return d
}

func TestImportarComChavesEhIdempotente(t *testing.T) {
	f := novoFake(t)
	p := novoProv(f, "dev")
	d := dongle(t)
	hx := hex.EncodeToString(d.DevID)
	r1, err := p.ImportarComChaves(context.Background(), d, "teste")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Schema != SchemaRegistro || r1.DevID != hx || r1.Perfil != "hmac" || r1.KMSChaveAuth != hx || r1.KMSChaveRec != hx || r1.Descricao != "teste" {
		t.Fatalf("%+v", r1)
	}
	if len(f.importado) != 2 {
		t.Fatalf("esperava 2 importações, houve %v", f.importado)
	}
	// reexecução: pula o import e reemite o mesmo registro
	r2, err := p.ImportarComChaves(context.Background(), d, "teste")
	if err != nil || r2 != r1 || len(f.importado) != 2 {
		t.Fatalf("%v %+v importações=%v", err, r2, f.importado)
	}
}

func TestImportacaoInterrompidaCompletaOQueFalta(t *testing.T) {
	f := novoFake(t)
	p := novoProv(f, "dev")
	d := dongle(t)
	f.chaves["transit-auth/"+hex.EncodeToString(d.DevID)] = true // só K_auth chegou a ser importada
	if _, err := p.ImportarComChaves(context.Background(), d, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.importado) != 1 || !strings.HasPrefix(f.importado[0], "transit-rec/") {
		t.Fatalf("deveria importar só K_rec: %v", f.importado)
	}
}

func TestNovoEscolheDevIDLivreSoNoVault(t *testing.T) {
	f := novoFake(t)
	p := novoProv(f, "dev")
	d, r, err := p.Novo(context.Background(), "hmac")
	if err != nil {
		t.Fatal(err)
	}
	if r.DevID != hex.EncodeToString(d.DevID) || len(d.KAuth) != 32 || len(d.KRec) != 32 || !f.chaves["transit-auth/"+r.DevID] || !f.chaves["transit-rec/"+r.DevID] {
		t.Fatalf("%+v %v", r, f.chaves)
	}
	if _, _, err := novoProv(f, "producao").Novo(context.Background(), "hmac"); err == nil {
		t.Fatal("novo deve ser recusado em producao")
	}
}

func TestImportarCSV(t *testing.T) {
	f := novoFake(t)
	p := novoProv(f, "dev")
	csv := filepath.Join(t.TempDir(), "d.csv")
	linha := func(dev string) string {
		return dev + "," + strings.Repeat("11", 32) + "," + strings.Repeat("22", 32) + ",hmac\n"
	}
	_ = os.WriteFile(csv, []byte("# comentário\n"+linha("0011223344556677")+linha("0011223344556678")), 0o600)
	ds, rs, err := p.ImportarCSV(context.Background(), csv)
	if err != nil || len(ds) != 2 || len(rs) != 2 || len(f.importado) != 4 {
		t.Fatalf("%v %d %d %v", err, len(ds), len(rs), f.importado)
	}
	if _, rs2, err := p.ImportarCSV(context.Background(), csv); err != nil || len(rs2) != 2 || len(f.importado) != 4 {
		t.Fatalf("reexecução deveria ser idempotente: %v %v", err, f.importado)
	}
	if _, _, err := novoProv(f, "producao").ImportarCSV(context.Background(), csv); err == nil {
		t.Fatal("CSV de teste é recusado em producao")
	}
}

// registroDourado é o registro.json que o backend (`tccelta provision register`) deve aceitar;
// o teste correspondente no backend usa o mesmo texto (contrato em docs/registro-dongle.md).
const registroDourado = `{
  "schema": "tccelta.dongle.registro.v1",
  "dev_id": "0011223344556677",
  "perfil": "hmac",
  "kms_chave_auth": "0011223344556677",
  "kms_chave_rec": "0011223344556677",
  "descricao": "dongle de teste"
}
`

func TestRegistroGravadoNoFormatoDoContrato(t *testing.T) {
	r := Registro{Schema: SchemaRegistro, DevID: "0011223344556677", Perfil: "hmac", KMSChaveAuth: "0011223344556677",
		KMSChaveRec: "0011223344556677", Descricao: "dongle de teste"}
	dir := t.TempDir()
	caminho, err := r.Gravar(dir)
	if err != nil {
		t.Fatal(err)
	}
	if caminho != filepath.Join(dir, "0011223344556677", "registro.json") {
		t.Fatal(caminho)
	}
	got, _ := os.ReadFile(caminho)
	if string(got) != registroDourado {
		t.Fatalf("registro.json difere do contrato:\n%s", got)
	}
}

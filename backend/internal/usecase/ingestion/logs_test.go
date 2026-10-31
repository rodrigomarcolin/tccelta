package ingestion

import (
	"errors"
	"testing"

	ding "tccelta/backend/internal/domain/ingestion"
)

func TestLogsBatchWithInvalidHMACTag(t *testing.T) {
	a := newTest(t)
	if _, err := a.send(t, batch(block{seq: 1, boot: 1}, block{seq: 2, boot: 1, bad: true}, block{seq: 3, boot: 1})); err != nil {
		t.Fatal(err)
	}
	acc := a.logged(t, "bloco.aceito")
	if len(acc) != 1 || acc[0]["seq"] != float64(1) || acc[0]["level"] != "DEBUG" {
		t.Fatalf("bloco.aceito: %v", acc)
	}
	bad := a.logged(t, "bloco.tag_invalida")
	if len(bad) != 1 || bad[0]["seq"] != float64(2) || bad[0]["verificacao"] != "hmac_k_rec_kms" ||
		bad[0]["perfil"] != "hmac" || bad[0]["level"] != "WARN" {
		t.Fatalf("bloco.tag_invalida: %v", bad)
	}
	lot := a.logged(t, "lote.recebido")
	if len(lot) != 1 {
		t.Fatalf("lote.recebido: %v", lot)
	}
	l := lot[0]
	if l["blocos"] != float64(3) || l["seq_min"] != float64(1) || l["seq_max"] != float64(3) || l["aceitos"] != float64(1) ||
		l["recusados"] != float64(1) || l["ack_anterior"] != float64(0) || l["ack"] != float64(1) || l["status"] != "interrompido" {
		t.Fatalf("lote.recebido: %v", l)
	}
	if len(a.logged(t, "lote.interrompido")) != 1 {
		t.Fatal("lote.interrompido continua sendo registrado")
	}
}

func TestLogsInvalidECDSASignature(t *testing.T) {
	a := newTest(t)
	k := asymKey(t, a)
	if _, err := a.send(t, asymBatch(k, block{seq: 1, boot: 1, bad: true})); err != nil {
		t.Fatal(err)
	}
	bad := a.logged(t, "bloco.tag_invalida")
	if len(bad) != 1 || bad[0]["verificacao"] != "ecdsa_p256_local" || bad[0]["perfil"] != "asym" ||
		bad[0]["dongle"] != asymDev.Hex() || bad[0]["tag_bytes"] != float64(64) {
		t.Fatalf("bloco.tag_invalida: %v", bad)
	}
	if lot := a.logged(t, "lote.recebido"); len(lot) != 1 || lot[0]["status"] != "sem_progresso" {
		t.Fatalf("lote.recebido: %v", lot)
	}
}

func TestLogsRejectedBlockOtherReason(t *testing.T) {
	a := newTest(t)
	if _, err := a.send(t, batch(block{seq: 1, boot: 1}, block{seq: 2, boot: 1, pid: 0x99})); err != nil {
		t.Fatal(err)
	}
	rej := a.logged(t, "bloco.recusado")
	if len(rej) != 1 || rej[0]["motivo"] != string(ding.ReasonUnknownPID) || rej[0]["seq"] != float64(2) {
		t.Fatalf("bloco.recusado: %v", rej)
	}
	if len(a.logged(t, "bloco.tag_invalida")) != 0 {
		t.Fatal("PID desconhecido não é tag inválida")
	}
}

func TestLogsMalformedBatch(t *testing.T) {
	a := newTest(t)
	if _, err := a.send(t, []byte{0xFF, 0x00}); !errors.Is(err, ding.ErrMalformedBatch) {
		t.Fatal(err)
	}
	m := a.logged(t, "lote.malformado")
	if len(m) != 1 || m[0]["dongle"] != dev.Hex() || m[0]["erro"] == "" {
		t.Fatalf("lote.malformado: %v", m)
	}
	if len(a.logged(t, "lote.recebido")) != 0 {
		t.Fatal("lote malformado não chega a ser processado")
	}
}

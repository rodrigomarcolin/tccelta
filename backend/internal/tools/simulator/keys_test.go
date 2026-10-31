package simulator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tccelta/backend/internal/domain/protocol"
)

const (
	kDev = "001122334455667a"
	kKA  = "131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132"
	kKR  = "535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f707172"
	kSig = "2222222222222222222222222222222222222222222222222222222222222222"
)

func csvFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "k.csv")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadKeysCSVFourAndFiveColumns(t *testing.T) {
	ks, err := ReadKeysCSV(csvFile(t, "# c\n"+
		"0011223344556677,"+kKA+","+kKR+",hmac\n"+
		kDev+","+kKA+","+kKR+",asym,"+kSig+"\n"+
		"0011223344556678,"+kKA+","+kKR+",hmac,\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ks) != 3 || ks[0].Profile.Name != "hmac" || ks[0].KSig != nil || ks[1].Profile.Name != "asym" || ks[1].KSig == nil ||
		ks[2].Profile.Name != "hmac" {
		t.Fatalf("%+v", ks)
	}
}

func TestReadKeysCSVRejectsAsymWithoutKSig(t *testing.T) {
	for _, body := range []string{
		kDev + "," + kKA + "," + kKR + ",asym\n",
		kDev + "," + kKA + "," + kKR + ",asym,\n",
		kDev + "," + kKA + "," + kKR + ",asym,22\n",
		kDev + "," + kKA + "," + kKR + ",xyz\n",
		kDev + "," + kKA + "\n",
	} {
		if _, err := ReadKeysCSV(csvFile(t, body)); err == nil {
			t.Errorf("deveria recusar %q", strings.TrimSpace(body))
		}
	}
}

func TestAsymDongleSignsVerifiableBlocks(t *testing.T) {
	ks, err := ReadKeysCSV(csvFile(t, kDev+","+kKA+","+kKR+",asym,"+kSig+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	d := NewDongle(ks[0], ConfigDongle{})
	d.GenerateBlocks(3)
	pub, err := protocol.ParsePublicKey(append([]byte{0x04}, append(ks[0].KSig.X.FillBytes(make([]byte, 32)), ks[0].KSig.Y.FillBytes(make([]byte, 32))...)...))
	if err != nil {
		t.Fatal(err)
	}
	tlvs := d.ReadForSending(10)
	if len(tlvs) != 4 { // BOOT + 3 blocos
		t.Fatalf("%d TLVs", len(tlvs))
	}
	for i, tlv := range tlvs {
		tl, _, err := protocol.ReadTLV(tlv)
		if err != nil {
			t.Fatal(err)
		}
		b, err := protocol.DecodeBlock(tl.V, protocol.ProfileAsym)
		if err != nil {
			t.Fatalf("bloco %d: %v", i, err)
		}
		if !protocol.VerifyBlockSignature(pub, protocol.TagInput(protocol.BlockType, ks[0].Dev, b.Data), b.Tag) {
			t.Fatalf("bloco %d: assinatura não verifica", i)
		}
	}
}

func TestNoneDongleSendsEmptyTag(t *testing.T) {
	d := NewDongle(Keys{Profile: protocol.ProfileNone}, ConfigDongle{})
	d.GenerateBlocks(1)
	for _, tlv := range d.ReadForSending(2) {
		tl, _, _ := protocol.ReadTLV(tlv)
		if b, err := protocol.DecodeBlock(tl.V, protocol.ProfileNone); err != nil || len(b.Tag) != 0 {
			t.Fatalf("%v %x", err, b.Tag)
		}
	}
}

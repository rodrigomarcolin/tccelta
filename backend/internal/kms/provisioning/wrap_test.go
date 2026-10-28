package provisioning

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestKWPRFC5649(t *testing.T) {
	kek, _ := hex.DecodeString("5840df6e29b02af1ab493b705bf16ea1ae8338f4dcc176a8")
	cases := []struct{ key, expected string }{
		{"c37b7e6492584340bed12207808941155068f738", "138bdeaa9b8fa7fc61f97742e72248ee5ae6ae5360d1ae6a5f54f373fa543b6a"},
		{"466f7250617369", "afbeb0f07dfbf5419200f2ccb50bb24f"},
	}
	for _, c := range cases {
		key, _ := hex.DecodeString(c.key)
		got, err := aesKWP(kek, key)
		if err != nil || hex.EncodeToString(got) != c.expected {
			t.Fatalf("%x %v", got, err)
		}
		back, err := aesKWPUnwrap(kek, got)
		if err != nil || !bytes.Equal(back, key) {
			t.Fatal("ida e volta", err)
		}
	}
}

func TestKWP32Bytes(t *testing.T) {
	kek := make([]byte, 32)
	key := make([]byte, 32)
	_, _ = rand.Read(kek)
	_, _ = rand.Read(key)
	w, err := aesKWP(kek, key)
	if err != nil || len(w) != 40 {
		t.Fatalf("len=%d %v", len(w), err)
	}
	back, err := aesKWPUnwrap(kek, w)
	if err != nil || !bytes.Equal(back, key) {
		t.Fatal(err)
	}
}

func TestWrapFormat(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{7}, 32)
	b64, err := Wrap(&priv.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(b64)
	k := priv.PublicKey.Size()
	ke, err := rsa.DecryptOAEP(sha256.New(), nil, priv, raw[:k], nil)
	if err != nil || len(ke) != 32 {
		t.Fatal(err)
	}
	back, err := aesKWPUnwrap(ke, raw[k:])
	if err != nil || !bytes.Equal(back, key) {
		t.Fatal("conteúdo")
	}
}

package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

func pemDecode(t *testing.T, s string) (*pem.Block, []byte) {
	b, rest := pem.Decode([]byte(s))
	if b == nil {
		t.Fatal("bad PEM")
	}
	return b, rest
}
func x509ParsePKIX(b *pem.Block) (ed25519.PublicKey, error) {
	k, err := x509.ParsePKIXPublicKey(b.Bytes)
	if err != nil {
		return nil, err
	}
	return k.(ed25519.PublicKey), nil
}
func b64Decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
func ed25519Verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	return ed25519.Verify(pub, msg, sig)
}

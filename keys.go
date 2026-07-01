package main

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
)

// SigningKeys holds the Ed25519 keypair used to sign quotes and callbacks. The
// platform verifies signatures against the public key registered under Kid.
type SigningKeys struct {
	Private ed25519.PrivateKey
	// PublicPEM is the SPKI/PKIX PEM the platform registry stores.
	PublicPEM string
	Kid       string
}

// EncKeys holds the X25519 keypair for the sealed-box user-context flow. The
// shell seals the userId to PublicB64; only the holder of Private can open it.
// The format is byte-compatible with libsodium crypto_box_seal (what the TS
// reference service and the cabinet shell use).
type EncKeys struct {
	Private *ecdh.PrivateKey
	Public  *ecdh.PublicKey
	// PublicB64 is the raw 32-byte X25519 public key, standard base64 — the same
	// encoding the platform registry and the shell exchange.
	PublicB64 string
}

// loadSigningKeys reads the Ed25519 key from PRIVATE_KEY_PEM (inline) or the
// PEM file at path, generating one on first run if the file is missing.
func loadSigningKeys(path, kid string) (SigningKeys, error) {
	if inline := os.Getenv("PRIVATE_KEY_PEM"); inline != "" {
		return signingFromPEM([]byte(inline), kid)
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := generateSigningPEM(path); err != nil {
			return SigningKeys{}, err
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return SigningKeys{}, fmt.Errorf("read signing key: %w", err)
	}
	return signingFromPEM(raw, kid)
}

func signingFromPEM(pemBytes []byte, kid string) (SigningKeys, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return SigningKeys{}, errors.New("signing key: not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return SigningKeys{}, fmt.Errorf("parse PKCS8: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return SigningKeys{}, errors.New("signing key: not Ed25519")
	}
	pubPEM, err := publicPEMFromEd25519(priv.Public().(ed25519.PublicKey))
	if err != nil {
		return SigningKeys{}, err
	}
	return SigningKeys{Private: priv, PublicPEM: pubPEM, Kid: kid}, nil
}

func generateSigningPEM(path string) error {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	buf := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return os.WriteFile(path, buf, 0o600)
}

func publicPEMFromEd25519(pub ed25519.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

// loadEncKeys reads the X25519 private key from ENC_PRIVATE_KEY_B64 (inline) or
// the base64 file at privPath, generating a keypair on first run. Both files
// hold raw 32-byte keys in standard base64.
func loadEncKeys(privPath, pubPath string) (EncKeys, error) {
	if inline := os.Getenv("ENC_PRIVATE_KEY_B64"); inline != "" {
		return encFromPrivB64(strings.TrimSpace(inline))
	}
	if _, err := os.Stat(privPath); errors.Is(err, os.ErrNotExist) {
		if err := generateEncKeys(privPath, pubPath); err != nil {
			return EncKeys{}, err
		}
	}
	raw, err := os.ReadFile(privPath)
	if err != nil {
		return EncKeys{}, fmt.Errorf("read enc key: %w", err)
	}
	return encFromPrivB64(strings.TrimSpace(string(raw)))
}

func encFromPrivB64(b64 string) (EncKeys, error) {
	seed, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return EncKeys{}, fmt.Errorf("enc key: not base64: %w", err)
	}
	priv, err := ecdh.X25519().NewPrivateKey(seed)
	if err != nil {
		return EncKeys{}, fmt.Errorf("enc key: invalid X25519 scalar: %w", err)
	}
	pub := priv.PublicKey()
	return EncKeys{
		Private:   priv,
		Public:    pub,
		PublicB64: base64.StdEncoding.EncodeToString(pub.Bytes()),
	}, nil
}

func generateEncKeys(privPath, pubPath string) error {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	privB64 := base64.StdEncoding.EncodeToString(priv.Bytes())
	pubB64 := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())
	if err := os.WriteFile(privPath, []byte(privB64), 0o600); err != nil {
		return err
	}
	return os.WriteFile(pubPath, []byte(pubB64), 0o644)
}

package main

import (
	"os"
	"testing"
)

// TestDecryptSealedFromLibsodium checks that a sealed box produced by libsodium
// (via SEALED_B64 env, sealed to the key in keys/enc.public.key) opens with
// decryptSealed. Skips if SEALED_B64 is unset. Run by the cross-lang harness.
func TestDecryptSealedFromLibsodium(t *testing.T) {
	sealed := os.Getenv("SEALED_B64")
	want := os.Getenv("WANT_USERID")
	if sealed == "" || want == "" {
		t.Skip("SEALED_B64/WANT_USERID not set")
	}
	enc, err := loadEncKeys("keys/enc.private.key", "keys/enc.public.key")
	if err != nil {
		t.Fatalf("load enc keys: %v", err)
	}
	got, err := decryptSealed(enc, sealed)
	if err != nil {
		t.Fatalf("decryptSealed: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestQuoteSelfVerify signs a quote and verifies it exactly as the platform
// does: ed25519.Verify over json.Marshal(quote with Sig="") against the SPKI
// public key. This is the same code path saga.VerifyQuoteSignature runs, so a
// pass here means the platform will accept our quotes.
func TestQuoteSelfVerify(t *testing.T) {
	sign, err := loadSigningKeys("keys/service.private.pem", "coffee-svc-key-1")
	if err != nil {
		t.Fatalf("load signing: %v", err)
	}
	q := Quote{
		Version: 1, ServiceID: "svc_x", UserID: "usr_1", Amount: "3.50",
		CurrencyID: 1, AcceptedCurrencyIDs: []int64{1}, Description: "Latte",
		Metadata: map[string]string{"productId": "latte"}, Nonce: "n1", Exp: 99,
	}
	signed, err := signQuote(q, sign.Private, sign.Kid)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Reproduce the platform verify path.
	msg, _ := canonicalQuoteBytes(signed)
	block, _ := pemDecode(t, sign.PublicPEM)
	pub, err := x509ParsePKIX(block)
	if err != nil {
		t.Fatalf("parse pub: %v", err)
	}
	sig, _ := b64Decode(signed.Sig)
	if !ed25519Verify(pub, msg, sig) {
		t.Fatal("platform-style verify FAILED")
	}
}

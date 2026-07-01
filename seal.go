package main

import (
	"encoding/base64"
	"errors"

	"golang.org/x/crypto/nacl/box"
)

// decryptSealed opens a libsodium crypto_box_seal (anonymous sealed box) that
// the cabinet shell produced against this service's X25519 public key. Go's
// nacl/box.OpenAnonymous implements the identical construction (X25519 +
// XSalsa20-Poly1305, nonce = BLAKE2b(ephemeralPub||recipientPub)), so it opens
// ciphertext sealed by libsodium and vice versa. Returns the plaintext userId.
func decryptSealed(keys EncKeys, encB64 string) (string, error) {
	sealed, err := base64.StdEncoding.DecodeString(encB64)
	if err != nil {
		return "", errors.New("ciphertext not base64")
	}
	var pub, priv [32]byte
	copy(pub[:], keys.Public.Bytes())
	copy(priv[:], keys.Private.Bytes())

	opened, ok := box.OpenAnonymous(nil, sealed, &pub, &priv)
	if !ok {
		// Forged/tampered ciphertext, or one sealed to a different key.
		return "", errors.New("invalid encrypted user id")
	}
	return string(opened), nil
}

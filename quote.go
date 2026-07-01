package main

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

// Quote mirrors the platform's saga.Quote field-for-field (same json tags and
// declaration order). The platform verifies the signature over
// json.Marshal(quote) with Sig blanked; because this struct marshals to the
// exact same bytes, we sign the same bytes — no manual canonicalization needed
// (unlike the TS reference service, which reimplements Go's JSON encoder).
type Quote struct {
	Version             int               `json:"version"`
	ServiceID           string            `json:"serviceId"`
	UserID              string            `json:"userId"`
	Amount              string            `json:"amount"`
	CurrencyID          int64             `json:"currencyId"`
	AcceptedCurrencyIDs []int64           `json:"acceptedCurrencyIds"`
	Description         string            `json:"description"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	Nonce               string            `json:"nonce"`
	Exp                 int64             `json:"exp"`
	Kid                 string            `json:"kid"`
	Sig                 string            `json:"sig"`
}

// Callback mirrors the platform's saga.Callback (async finalization).
type Callback struct {
	OrderID     string `json:"orderId"`
	Status      string `json:"status"`
	ExternalRef string `json:"externalRef"`
	Kid         string `json:"kid"`
	Sig         string `json:"sig"`
}

// canonicalQuoteBytes is the signed form: the quote with Sig blanked, marshaled
// the same way the platform does.
func canonicalQuoteBytes(q Quote) ([]byte, error) {
	unsigned := q
	unsigned.Sig = ""
	return json.Marshal(unsigned)
}

func canonicalCallbackBytes(c Callback) ([]byte, error) {
	unsigned := c
	unsigned.Sig = ""
	return json.Marshal(unsigned)
}

// signQuote populates Kid and Sig (base64 Ed25519 over the canonical bytes).
func signQuote(q Quote, priv ed25519.PrivateKey, kid string) (Quote, error) {
	q.Kid = kid
	q.Sig = ""
	msg, err := canonicalQuoteBytes(q)
	if err != nil {
		return Quote{}, err
	}
	sig, err := priv.Sign(nil, msg, crypto.Hash(0))
	if err != nil {
		return Quote{}, err
	}
	q.Sig = base64.StdEncoding.EncodeToString(sig)
	return q, nil
}

// signCallback populates Kid and Sig for an async finalization callback.
func signCallback(c Callback, priv ed25519.PrivateKey, kid string) (Callback, error) {
	c.Kid = kid
	c.Sig = ""
	msg, err := canonicalCallbackBytes(c)
	if err != nil {
		return Callback{}, err
	}
	sig, err := priv.Sign(nil, msg, crypto.Hash(0))
	if err != nil {
		return Callback{}, err
	}
	c.Sig = base64.StdEncoding.EncodeToString(sig)
	return c, nil
}

// quoteHash is the hex sha256 of the canonical quote bytes (parity with the
// platform's QuoteHash; unused by the shell in CONSENT_MODE=none but kept for
// completeness).
func quoteHash(q Quote) (string, error) {
	msg, err := canonicalQuoteBytes(q)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(msg)
	return hex.EncodeToString(sum[:]), nil
}

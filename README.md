# coffee-service

A reference **Service Constructor** backend, written in **Go**, that sells coffee
from a fixed menu. It's the sibling of the TypeScript `example-service` — the two
implement the *same* platform contract in different languages, proving the
Service Constructor platform is language-agnostic.

## What it demonstrates

- **Quote signing (Ed25519).** Signs quotes with the same bytes the Go platform
  verifies: `json.Marshal(quote)` with `sig` blanked. Because `Quote` here has
  the same JSON tags/field order as the platform's `saga.Quote`, no manual
  canonicalization is needed (unlike the TS service, which reimplements Go's JSON
  encoder in `canonical.ts`).
- **Sealed-box identity (X25519).** Opens the cabinet shell's sealed userId with
  `golang.org/x/crypto/nacl/box.OpenAnonymous`, which is byte-compatible with
  libsodium `crypto_box_seal` (what the shell and TS service use).
- **The saga contract:** `/execute` (idempotent), `/status` (reconciler),
  `/decrypt-user`, `/healthz`.

## Endpoints

| Method | Path                | Purpose                                         |
|--------|---------------------|-------------------------------------------------|
| POST   | `/quote`            | Issue a signed quote for a menu item            |
| POST   | `/execute`          | Fulfill the order (idempotent by `orderId`)     |
| GET    | `/status/{orderId}` | Canonical status for the reconciler             |
| GET    | `/orders?userId=`   | A user's past orders (mini-app history)          |
| POST   | `/decrypt-user`     | Open the shell's sealed userId                  |
| GET    | `/menu`             | The coffee menu (id, title, emoji, price)       |
| GET    | `/healthz`          | Liveness + the service's X25519 public key      |

## Run

```sh
go build -o coffee-service .
SERVICE_ID=svc_xxx \
SERVICE_KID=coffee-svc-key-1 \
PORT=4100 \
PLATFORM_BASE_URL=http://localhost:8080 \
./coffee-service
```

Keys are generated on first run into `keys/` (Ed25519 signing PEM + X25519 enc
keypair). Register the printed public keys with the platform (the `encPubKey`
log line and `keys/service.public.pem`). Env overrides: `PRIVATE_KEY_PEM`,
`ENC_PRIVATE_KEY_B64`, `PRIVATE_KEY_PATH`, `ENC_PRIVATE_KEY_PATH`.

The `coffee-miniapp/` frontend proxies `/service/*` to this backend.

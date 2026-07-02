// coffee-service is a reference Service Constructor backend, in Go, that sells
// coffee from a fixed menu. It demonstrates the service side of the platform
// contract (white paper §13) — the same contract the TS example-service
// implements, proving the platform is language-agnostic:
//
//	POST /quote           -> issue a signed quote (Ed25519)
//	POST /execute         -> fulfill the order (idempotent by orderId)
//	GET  /status/:orderId -> canonical status for the reconciler
//	GET  /orders?userId=  -> a user's past purchases (mini-app history)
//	POST /decrypt-user    -> open the shell's sealed userId (X25519 sealed box)
//	GET  /healthz         -> liveness + the service's enc public key
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type server struct {
	serviceID     string
	platformBase  string
	quoteTTL      int64
	acceptedCurrs []int64
	sign          SigningKeys
	enc           EncKeys
	store         *store
}

func main() {
	serviceID := os.Getenv("SERVICE_ID")
	kid := envOr("SERVICE_KID", "coffee-svc-key-1")
	port := envOr("PORT", "4100")
	platformBase := envOr("PLATFORM_BASE_URL", "http://localhost:8080")

	sign, err := loadSigningKeys(envOr("PRIVATE_KEY_PATH", "keys/service.private.pem"), kid)
	if err != nil {
		log.Fatalf("load signing keys: %v", err)
	}
	enc, err := loadEncKeys(
		envOr("ENC_PRIVATE_KEY_PATH", "keys/enc.private.key"),
		envOr("ENC_PUBLIC_KEY_PATH", "keys/enc.public.key"),
	)
	if err != nil {
		log.Fatalf("load enc keys: %v", err)
	}

	s := &server{
		serviceID:     serviceID,
		platformBase:  platformBase,
		quoteTTL:      300,
		acceptedCurrs: []int64{1},
		sign:          sign,
		enc:           enc,
		store:         newStore(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /quote", s.handleQuote)
	mux.HandleFunc("POST /execute", s.handleExecute)
	mux.HandleFunc("GET /status/{orderId}", s.handleStatus)
	mux.HandleFunc("GET /orders", s.handleOrders)
	mux.HandleFunc("POST /decrypt-user", s.handleDecryptUser)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /menu", s.handleMenu)
	mux.HandleFunc("GET /scenarios", s.handleScenarios)

	log.Printf("coffee-service on :%s", port)
	log.Printf("  serviceId: %s", orNotSet(serviceID))
	log.Printf("  kid:       %s", kid)
	log.Printf("  platform:  %s", platformBase)
	log.Printf("  encPubKey: %s", enc.PublicB64)
	if err := http.ListenAndServe(":"+port, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

// --- POST /quote -----------------------------------------------------------
// The mini-app calls this to obtain a signed quote for a menu item, then hands
// it to the wallet shell via the bridge. In production userId comes from the
// verified platform session; here it is supplied directly for the demo.
func (s *server) handleQuote(w http.ResponseWriter, r *http.Request) {
	if s.serviceID == "" {
		writeErr(w, 500, "SERVICE_ID not configured")
		return
	}
	var body struct {
		UserID    string `json:"userId"`
		ProductID string `json:"productId"`
		Scenario  string `json:"scenario"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if strings.TrimSpace(body.UserID) == "" {
		writeErr(w, 400, "userId is required")
		return
	}
	product, ok := productByID(body.ProductID)
	if !ok {
		writeErr(w, 400, "unknown productId")
		return
	}

	// The mini-app picks a saga demo scenario; it rides in the quote metadata and
	// the platform echoes it back to /execute so we can drive every saga path.
	sc := resolveScenario(body.Scenario)

	q := Quote{
		Version:             1,
		ServiceID:           s.serviceID,
		UserID:              body.UserID,
		Amount:              product.Price,
		CurrencyID:          product.CurrencyID,
		AcceptedCurrencyIDs: s.acceptedCurrs,
		Description:         product.Title + " · " + sc.Title,
		Metadata:            map[string]string{"productId": product.ID, "scenario": sc.ID},
		Nonce:               newNonce(),
		Exp:                 time.Now().Unix() + s.quoteTTL,
	}
	signed, err := signQuote(q, s.sign.Private, s.sign.Kid)
	if err != nil {
		writeErr(w, 500, "sign failed")
		return
	}
	writeJSON(w, 200, map[string]any{"quote": signed})
}

// --- POST /execute ---------------------------------------------------------
// Called by the platform orchestrator to fulfill the order. Idempotent by
// orderId: a repeated call returns the same result and never re-fulfills.
func (s *server) handleExecute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderID  string            `json:"orderId"`
		UserID   string            `json:"userId"`
		Metadata map[string]string `json:"metadata"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if strings.TrimSpace(body.OrderID) == "" {
		writeJSON(w, 400, map[string]any{"status": "FAILED", "reason": "missing orderId"})
		return
	}
	// Idempotency: a resolved order returns its verdict; PENDING keeps waiting.
	if prior, ok := s.store.get(body.OrderID); ok && prior.Status != "NOT_DONE" {
		writeJSON(w, 200, map[string]any{"status": statusToProvider(prior.Status), "externalRef": prior.ExternalRef})
		return
	}

	product, ok := productByID(body.Metadata["productId"])
	if !ok {
		product = menu[0]
	}
	sc := resolveScenario(body.Metadata["scenario"])
	externalRef := "cup_" + newNonce()

	switch sc.Kind {
	case kindSync:
		if sc.Verdict == "FAILED" {
			s.store.put(Delivery{OrderID: body.OrderID, Status: "NOT_DONE", UserID: body.UserID, ProductID: product.ID, Title: product.Title, ExternalRef: externalRef, Scenario: sc.ID, CreatedAt: time.Now().UnixMilli()})
			writeJSON(w, 200, map[string]any{"status": "FAILED", "reason": "scenario: sync failure"})
			return
		}
		s.deliver(body.OrderID, product, body.UserID, externalRef, sc.ID)
		writeJSON(w, 200, map[string]any{"status": "SUCCESS", "externalRef": externalRef})

	case kindRetry:
		// Fail the first FailN calls with 503 so the platform HTTP executor
		// retries with backoff. FailN=-1 (retry-exhausted) never succeeds and the
		// executor eventually gives up -> platform compensates (refund).
		attempt := s.store.bumpAttempts(body.OrderID, sc.ID)
		if sc.FailN < 0 || attempt <= sc.FailN {
			writeJSON(w, 503, map[string]any{"status": "FAILED", "reason": "scenario: retry"})
			return
		}
		s.deliver(body.OrderID, product, body.UserID, externalRef, sc.ID)
		writeJSON(w, 200, map[string]any{"status": "SUCCESS", "externalRef": externalRef})

	case kindAsyncCallback:
		// Park PENDING, then finalize via a signed webhook (success or fail).
		s.store.put(Delivery{OrderID: body.OrderID, Status: "PENDING", UserID: body.UserID, ProductID: product.ID, Title: product.Title, ExternalRef: externalRef, Scenario: sc.ID, CreatedAt: time.Now().UnixMilli()})
		go s.scheduleCallback(body.OrderID, product, body.UserID, externalRef, sc.Verdict)
		writeJSON(w, 200, map[string]any{"status": "PENDING", "externalRef": externalRef})

	case kindAsyncReconcile:
		// Park PENDING and send NO webhook. The order resolves only when the
		// reconciler queries /status, which reports sc.Reconile.
		s.store.put(Delivery{OrderID: body.OrderID, Status: "PENDING", UserID: body.UserID, ProductID: product.ID, Title: product.Title, ExternalRef: externalRef, Scenario: sc.ID, CreatedAt: time.Now().UnixMilli()})
		writeJSON(w, 200, map[string]any{"status": "PENDING", "externalRef": externalRef})
	}
}

// deliver records a successful fulfillment.
func (s *server) deliver(orderID string, p Product, userID, externalRef, scenario string) {
	s.store.put(Delivery{
		OrderID:     orderID,
		Status:      "DONE",
		UserID:      userID,
		ProductID:   p.ID,
		Title:       p.Title,
		ExternalRef: externalRef,
		Scenario:    scenario,
		CreatedAt:   time.Now().UnixMilli(),
	})
}

// --- GET /status/{orderId} -------------------------------------------------
// The reconciler queries this before any compensation. Returns the canonical
// status the platform maps onto DONE / NOT_DONE.
func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	d, ok := s.store.get(r.PathValue("orderId"))
	if !ok {
		writeJSON(w, 200, map[string]any{"status": "NOT_DONE"})
		return
	}
	// For async-reconcile scenarios the order sits PENDING with no webhook; the
	// reconciler drives the outcome purely from what /status reports here.
	if d.Status == "PENDING" && d.Scenario != "" {
		sc := resolveScenario(d.Scenario)
		if sc.Kind == kindAsyncReconcile && sc.Reconile != "" {
			// A DONE reconcile also fulfills the order so history shows it.
			if sc.Reconile == "DONE" {
				p, ok := productByID(d.ProductID)
				if !ok {
					p = menu[0]
				}
				s.deliver(d.OrderID, p, d.UserID, d.ExternalRef, d.Scenario)
			}
			writeJSON(w, 200, map[string]any{"status": sc.Reconile, "externalRef": d.ExternalRef})
			return
		}
	}
	writeJSON(w, 200, map[string]any{"status": d.Status, "externalRef": d.ExternalRef})
}

// --- GET /scenarios --------------------------------------------------------
// The mini-app fetches the scenario catalog to render its picker.
func (s *server) handleScenarios(w http.ResponseWriter, _ *http.Request) {
	list := make([]scenario, 0, len(scenarioOrder))
	for _, id := range scenarioOrder {
		list = append(list, scenarios[id])
	}
	writeJSON(w, 200, map[string]any{"scenarios": list, "default": defaultScenario})
}

// scheduleCallback simulates async provisioning: after a short delay it resolves
// the order and POSTs a SIGNED callback to the platform with the verdict.
func (s *server) scheduleCallback(orderID string, p Product, userID, externalRef, verdict string) {
	time.Sleep(callbackDelay())
	if verdict == "SUCCESS" {
		s.deliver(orderID, p, userID, externalRef, "async-success")
	} else {
		s.store.put(Delivery{OrderID: orderID, Status: "NOT_DONE", UserID: userID, ProductID: p.ID, Title: p.Title, ExternalRef: externalRef, Scenario: "async-fail", CreatedAt: time.Now().UnixMilli()})
	}
	cb, err := signCallback(Callback{OrderID: orderID, Status: verdict, ExternalRef: externalRef}, s.sign.Private, s.sign.Kid)
	if err != nil {
		log.Printf("[callback] order=%s sign failed: %v", orderID, err)
		return
	}
	buf, _ := json.Marshal(cb)
	resp, err := http.Post(s.platformBase+"/v1/services/callback", "application/json", bytes.NewReader(buf))
	if err != nil {
		log.Printf("[callback] order=%s post failed: %v", orderID, err)
		return
	}
	_ = resp.Body.Close()
	log.Printf("[callback] order=%s verdict=%s platform responded %d", orderID, verdict, resp.StatusCode)
}

func callbackDelay() time.Duration {
	if v := os.Getenv("ASYNC_DELAY_MS"); v != "" {
		if ms, err := time.ParseDuration(v + "ms"); err == nil {
			return ms
		}
	}
	return 1500 * time.Millisecond
}

// --- GET /orders?userId= ---------------------------------------------------
func (s *server) handleOrders(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	if userID == "" {
		writeErr(w, 400, "userId is required")
		return
	}
	writeJSON(w, 200, map[string]any{"orders": s.store.forUser(userID)})
}

// --- POST /decrypt-user ----------------------------------------------------
// The mini-app posts the shell's sealed userId here; only this service can open
// it, so the returned userId is the trusted identity.
func (s *server) handleDecryptUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EncUserID string `json:"encUserId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.EncUserID == "" {
		writeErr(w, 400, "encUserId is required")
		return
	}
	userID, err := decryptSealed(s.enc, body.EncUserID)
	if err != nil {
		writeErr(w, 400, "invalid encrypted user id")
		return
	}
	writeJSON(w, 200, map[string]any{"userId": userID})
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	ids := make([]string, 0, len(menu))
	for _, p := range menu {
		ids = append(ids, p.ID)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "products": ids, "encryptionPublicKey": s.enc.PublicB64})
}

func (s *server) handleMenu(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"menu": menu})
}

// --- helpers ---------------------------------------------------------------

// statusToProvider maps our internal delivery status to the provider status the
// platform executor understands on a repeated /execute call.
func statusToProvider(s string) string {
	switch s {
	case "DONE":
		return "SUCCESS"
	case "NOT_DONE":
		return "FAILED"
	default: // PENDING, UNKNOWN
		return "PENDING"
	}
}

func newNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func orNotSet(s string) string {
	if s == "" {
		return "(SERVICE_ID not set)"
	}
	return s
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

// withCORS allows the mini-app frontend (a different origin during dev) to call
// /quote, /decrypt-user, etc. from the browser.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

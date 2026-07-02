package main

import (
	"sync"
	"time"
)

// Delivery records a fulfilled (or pending) coffee order. It backs both
// idempotent /execute and the /status reconciler query. In-memory only — a real
// service would persist this.
type Delivery struct {
	OrderID     string `json:"orderId"`
	Status      string `json:"status"` // DONE | PENDING | NOT_DONE | UNKNOWN
	UserID      string `json:"userId"`
	ProductID   string `json:"productId"`
	Title       string `json:"title"`
	ExternalRef string `json:"externalRef"`
	// Scenario carried from the quote metadata, so /status and the async callback
	// know how this order is meant to resolve. Not serialized to the mini-app.
	Scenario string `json:"-"`
	// Attempts counts /execute calls for this order (retry scenarios).
	Attempts  int   `json:"-"`
	CreatedAt int64 `json:"createdAt"`
}

// store is a tiny concurrency-safe map of orderId -> Delivery.
type store struct {
	mu sync.RWMutex
	m  map[string]Delivery
}

func newStore() *store { return &store{m: make(map[string]Delivery)} }

func (s *store) get(orderID string) (Delivery, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.m[orderID]
	return d, ok
}

func (s *store) put(d Delivery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[d.OrderID] = d
}

// bumpAttempts increments (and returns) the execute attempt counter for an
// order, creating a placeholder record if this is the first attempt.
func (s *store) bumpAttempts(orderID, scenario string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[orderID]
	if !ok {
		d = Delivery{OrderID: orderID, Status: "NOT_DONE", Scenario: scenario, CreatedAt: time.Now().UnixMilli()}
	}
	d.Attempts++
	if d.Scenario == "" {
		d.Scenario = scenario
	}
	s.m[orderID] = d
	return d.Attempts
}

func (s *store) forUser(userID string) []Delivery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Delivery
	for _, d := range s.m {
		if d.UserID == userID {
			out = append(out, d)
		}
	}
	return out
}

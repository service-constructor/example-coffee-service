package main

import "sync"

// Delivery records a fulfilled (or pending) coffee order. It backs both
// idempotent /execute and the /status reconciler query. In-memory only — a real
// service would persist this.
type Delivery struct {
	OrderID     string `json:"orderId"`
	Status      string `json:"status"` // DONE | PENDING
	UserID      string `json:"userId"`
	ProductID   string `json:"productId"`
	Title       string `json:"title"`
	ExternalRef string `json:"externalRef"`
	CreatedAt   int64  `json:"createdAt"`
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

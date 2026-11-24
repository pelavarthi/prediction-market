package market

import (
	"fmt"
	"sync"
)

// Store is a thread-safe in-memory collection of markets.
type Store struct {
	mu      sync.RWMutex
	markets map[string]*Market
	order   []*Market
	nextID  int
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{markets: make(map[string]*Market)}
}

// Create adds a market to the store, giving it an ID if it has none.
func (s *Store) Create(m *Market) *Market {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.ID == "" {
		s.nextID++
		m.ID = fmt.Sprintf("mkt-%d", s.nextID)
	}
	if _, exists := s.markets[m.ID]; !exists {
		s.order = append(s.order, m)
	}
	s.markets[m.ID] = m
	return m
}

// Get returns the market with the given ID, or false if there is none.
func (s *Store) Get(id string) (*Market, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.markets[id]
	return m, ok
}

// List returns all markets in the order they were created.
func (s *Store) List() []*Market {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Market, len(s.order))
	copy(out, s.order)
	return out
}

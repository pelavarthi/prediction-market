package market

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// FeeRate is the fee charged on every buy, as a share of the trade cost.
// 1% of the notional goes to the house; the rest goes to the market maker.
const FeeRate = 0.01

// Order is one filled buy against the market maker.
type Order struct {
	ID         string
	UserID     string
	MarketID   string
	Outcome    Outcome
	Shares     float64
	Cost       float64 // what the market maker charged, before the fee
	Fee        float64 // FeeRate * Cost
	Total      float64 // Cost + Fee: what left the buyer's balance
	AvgPrice   float64 // Cost / Shares: what each share worked out at
	PriceAfter float64 // price of the outcome once the trade moved the line
	PlacedAt   time.Time
}

// Position is one user's holding in one market. AvgCostYes and AvgCostNo
// are the average price paid per share on each side, before fees — the
// cost basis a cash-out measures profit against.
type Position struct {
	MarketID   string
	Yes        float64
	No         float64
	AvgCostYes float64
	AvgCostNo  float64
}

// Shares returns the holding on one side of the market.
func (p Position) Shares(o Outcome) float64 {
	if o == Yes {
		return p.Yes
	}
	return p.No
}

// AvgCost returns the average price paid per share on one side.
func (p Position) AvgCost(o Outcome) float64 {
	if o == Yes {
		return p.AvgCostYes
	}
	return p.AvgCostNo
}

// Entry is one line in a user's balance ledger: every credit and debit, so
// the balance can always be rebuilt by summing the entries.
type Entry struct {
	Kind         string  // "deposit", "buy", "cashout", "fee"
	Amount       float64 // signed: deposits and cash-outs positive, buys and fees negative
	BalanceAfter float64 // balance right after this entry
	At           time.Time
}

// Account is one trader: a cash balance, a position per market, and the
// ledger of every balance change.
type Account struct {
	UserID    string
	Balance   float64
	Positions map[string]*Position // keyed by market ID
	Ledger    []Entry
}

// Ledger is where orders live: the accounts, the filled orders and
// cash-outs, and the store the markets sit in. One mutex serializes order
// placement, so the quote, the line move, and the balance debit happen as
// one step and two buyers can never trade on the same stale price.
type Ledger struct {
	mu       sync.Mutex
	store    *Store
	accounts map[string]*Account
	orders   []*Order
	cashouts []*CashOut
	nextID   int
}

// NewLedger returns an empty ledger over the given store.
func NewLedger(store *Store) *Ledger {
	return &Ledger{store: store, accounts: make(map[string]*Account)}
}

// CreateAccount opens an account with a starting balance. The opening
// balance is the first ledger entry, so the ledger always sums to the
// balance.
func (l *Ledger) CreateAccount(userID string, balance float64) (*Account, error) {
	if userID == "" {
		return nil, errors.New("market: user id must not be empty")
	}
	if balance < 0 {
		return nil, fmt.Errorf("market: starting balance must not be negative, got %.4g", balance)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.accounts[userID]; exists {
		return nil, fmt.Errorf("market: account %q already exists", userID)
	}
	a := &Account{UserID: userID, Balance: balance, Positions: make(map[string]*Position)}
	a.record("deposit", balance)
	l.accounts[userID] = a
	return a, nil
}

// Account returns the account for a user, or false if there is none.
func (l *Ledger) Account(userID string) (*Account, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.accounts[userID]
	return a, ok
}

// Deposit adds funds to an account.
func (l *Ledger) Deposit(userID string, amount float64) error {
	if amount <= 0 {
		return fmt.Errorf("market: deposit must be positive, got %.4g", amount)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.accounts[userID]
	if !ok {
		return fmt.Errorf("market: unknown user %q", userID)
	}
	a.Balance += amount
	a.record("deposit", amount)
	return nil
}

// PlaceBuy buys shares of an outcome for a user. It quotes the trade, then
// checks the market is open, the quoted price is in bounds, and the user can
// afford the cost plus the 1% fee. Only then does it move the line, debit
// the balance, and credit the position. A failed check changes nothing:
// the market does not move and the balance is untouched.
func (l *Ledger) PlaceBuy(userID, marketID string, o Outcome, shares float64) (*Order, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.accounts[userID]
	if !ok {
		return nil, fmt.Errorf("market: unknown user %q", userID)
	}
	m, ok := l.store.Get(marketID)
	if !ok {
		return nil, fmt.Errorf("market: unknown market %q", marketID)
	}
	if !m.Open {
		return nil, fmt.Errorf("market: market %q is closed", marketID)
	}
	q, err := m.QuoteBuy(o, shares)
	if err != nil {
		return nil, err
	}
	// The LMSR price always sits in (0, 1), so this is a guard on the
	// pricing engine, not a case that should ever fire.
	if q.AvgPrice <= 0 || q.AvgPrice >= 1 || q.PriceAfter <= 0 || q.PriceAfter >= 1 {
		return nil, fmt.Errorf("market: quoted price out of bounds (avg %.4f, after %.4f)", q.AvgPrice, q.PriceAfter)
	}
	fee := FeeRate * q.Cost
	total := q.Cost + fee
	if total > a.Balance {
		return nil, fmt.Errorf("market: insufficient funds: buy costs %.2f (%.2f + %.2f fee), balance is %.2f", total, q.Cost, fee, a.Balance)
	}
	if _, err := m.Buy(o, shares); err != nil {
		return nil, err // unreachable: the quote above already validated
	}

	l.nextID++
	order := &Order{
		ID:         fmt.Sprintf("ord-%d", l.nextID),
		UserID:     userID,
		MarketID:   marketID,
		Outcome:    o,
		Shares:     shares,
		Cost:       q.Cost,
		Fee:        fee,
		Total:      total,
		AvgPrice:   q.AvgPrice,
		PriceAfter: q.PriceAfter,
		PlacedAt:   time.Now().UTC(),
	}
	// Debit in two steps so the ledger shows the trade and the fee apart.
	a.Balance -= q.Cost
	a.record("buy", -q.Cost)
	a.Balance -= fee
	a.record("fee", -fee)

	pos := a.Positions[marketID]
	if pos == nil {
		pos = &Position{MarketID: marketID}
		a.Positions[marketID] = pos
	}
	// Fold the new shares into the average cost, so the position always
	// knows what its shares were bought for.
	switch o {
	case Yes:
		pos.AvgCostYes = (pos.Yes*pos.AvgCostYes + q.Cost) / (pos.Yes + shares)
		pos.Yes += shares
	case No:
		pos.AvgCostNo = (pos.No*pos.AvgCostNo + q.Cost) / (pos.No + shares)
		pos.No += shares
	}
	l.orders = append(l.orders, order)
	return order, nil
}

// Position returns a user's holding in a market, or false if they hold
// nothing there.
func (l *Ledger) Position(userID, marketID string) (Position, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.accounts[userID]
	if !ok {
		return Position{}, false
	}
	p, ok := a.Positions[marketID]
	if !ok {
		return Position{}, false
	}
	return *p, true
}

// Orders returns a user's filled orders, oldest first.
func (l *Ledger) Orders(userID string) []*Order {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []*Order
	for _, o := range l.orders {
		if o.UserID == userID {
			out = append(out, o)
		}
	}
	return out
}

func (a *Account) record(kind string, amount float64) {
	a.Ledger = append(a.Ledger, Entry{
		Kind:         kind,
		Amount:       amount,
		BalanceAfter: a.Balance,
		At:           time.Now().UTC(),
	})
}

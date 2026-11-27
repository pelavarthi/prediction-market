package market

import (
	"fmt"
	"time"
)

// CashOut is one sale of shares back to the market maker.
type CashOut struct {
	ID          string
	UserID      string
	MarketID    string
	Outcome     Outcome
	Shares      float64
	Proceeds    float64 // what the market maker paid, before the fee
	Fee         float64 // FeeRate * Proceeds
	Net         float64 // Proceeds - Fee: what landed in the balance
	AvgPrice    float64 // Proceeds / Shares: what each share sold for
	PriceAfter  float64 // price of the outcome once the sale moved the line
	CostBasis   float64 // what those shares originally cost, before fees
	RealizedPnL float64 // Proceeds - CostBasis: profit or loss, before fees
	CashedAt    time.Time
}

// CashOutQuote is the price of a cash-out that has not happened yet: the
// proceeds, the fee, and what would land in the balance.
type CashOutQuote struct {
	Outcome    Outcome
	Shares     float64
	Proceeds   float64
	Fee        float64
	Net        float64
	AvgPrice   float64
	PriceAfter float64
}

// QuoteCashOut prices selling shares back to the market maker without
// moving anything. It checks the user holds the shares and the market is
// open, so a quote is only ever for a cash-out that could actually happen.
func (l *Ledger) QuoteCashOut(userID, marketID string, o Outcome, shares float64) (CashOutQuote, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.accounts[userID]
	if !ok {
		return CashOutQuote{}, fmt.Errorf("market: unknown user %q", userID)
	}
	m, ok := l.store.Get(marketID)
	if !ok {
		return CashOutQuote{}, fmt.Errorf("market: unknown market %q", marketID)
	}
	if !m.Open {
		return CashOutQuote{}, fmt.Errorf("market: market %q is closed", marketID)
	}
	if held := a.held(marketID, o); held < shares {
		return CashOutQuote{}, fmt.Errorf("market: cannot cash out %.4g %s shares, position holds %.4g", shares, o, held)
	}
	q, err := m.QuoteSell(o, shares)
	if err != nil {
		return CashOutQuote{}, err
	}
	fee := FeeRate * q.Cost
	return CashOutQuote{
		Outcome:    o,
		Shares:     shares,
		Proceeds:   q.Cost,
		Fee:        fee,
		Net:        q.Cost - fee,
		AvgPrice:   q.AvgPrice,
		PriceAfter: q.PriceAfter,
	}, nil
}

// CashOut sells shares back to the market maker at the current price. It
// works like a buy in reverse: quote the sale, check the user holds the
// shares, move the line the other way, then credit the balance. A partial
// cash-out is just a smaller share count — the shares left behind keep
// their original cost basis, so the next cash-out still measures profit
// against what was actually paid.
func (l *Ledger) CashOut(userID, marketID string, o Outcome, shares float64) (*CashOut, error) {
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
	pos := a.Positions[marketID]
	if pos == nil || pos.Shares(o) < shares {
		return nil, fmt.Errorf("market: cannot cash out %.4g %s shares, position holds %.4g", shares, o, a.held(marketID, o))
	}
	q, err := m.QuoteSell(o, shares)
	if err != nil {
		return nil, err
	}
	// The LMSR price always sits in (0, 1), so this is a guard on the
	// pricing engine, not a case that should ever fire.
	if q.AvgPrice <= 0 || q.AvgPrice >= 1 || q.PriceAfter <= 0 || q.PriceAfter >= 1 {
		return nil, fmt.Errorf("market: quoted price out of bounds (avg %.4f, after %.4f)", q.AvgPrice, q.PriceAfter)
	}
	fee := FeeRate * q.Cost
	net := q.Cost - fee
	basis := pos.AvgCost(o) * shares
	if _, err := m.Sell(o, shares); err != nil {
		return nil, err // unreachable: the quote above already validated
	}

	switch o {
	case Yes:
		pos.Yes -= shares
	case No:
		pos.No -= shares
	}
	// Credit in two steps so the ledger shows the sale and the fee apart.
	a.Balance += q.Cost
	a.record("cashout", q.Cost)
	a.Balance -= fee
	a.record("fee", -fee)

	l.nextID++
	co := &CashOut{
		ID:          fmt.Sprintf("co-%d", l.nextID),
		UserID:      userID,
		MarketID:    marketID,
		Outcome:     o,
		Shares:      shares,
		Proceeds:    q.Cost,
		Fee:         fee,
		Net:         net,
		AvgPrice:    q.AvgPrice,
		PriceAfter:  q.PriceAfter,
		CostBasis:   basis,
		RealizedPnL: q.Cost - basis,
		CashedAt:    time.Now().UTC(),
	}
	l.cashouts = append(l.cashouts, co)
	return co, nil
}

// CashOuts returns a user's cash-outs, oldest first.
func (l *Ledger) CashOuts(userID string) []*CashOut {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []*CashOut
	for _, c := range l.cashouts {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return out
}

// held returns how many shares of an outcome a user holds in a market.
func (a *Account) held(marketID string, o Outcome) float64 {
	pos := a.Positions[marketID]
	if pos == nil {
		return 0
	}
	return pos.Shares(o)
}

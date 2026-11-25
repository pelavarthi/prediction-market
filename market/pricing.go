package market

import (
	"fmt"
	"math"
)

// Quote describes the price of one trade against the market maker: what it
// costs, what the line was before the trade, and where the trade moves it to.
// Cost is the total to pay (buy) or receive (sell), before any fee; fees
// live in the order flow, not in the pricing engine.
type Quote struct {
	Outcome     Outcome
	Shares      float64
	Cost        float64
	AvgPrice    float64 // Cost / Shares: what each share worked out at
	PriceBefore float64 // price of the outcome before the trade
	PriceAfter  float64 // price of the outcome after the trade
}

// Impact is how far the trade moves the line, in price points.
func (q Quote) Impact() float64 {
	return q.PriceAfter - q.PriceBefore
}

// QuoteBuy prices buying shares of an outcome without moving the market.
// The cost is C(q + x) - C(q): the market maker charges the difference in
// its cost function, so a big buy pays a worse average price than a small
// one because the line moves while the trade fills.
func (m *Market) QuoteBuy(o Outcome, shares float64) (Quote, error) {
	if err := m.checkTrade(o, shares); err != nil {
		return Quote{}, err
	}
	before := m.mustPrice(o)
	qYes, qNo := m.QYes, m.QNo
	switch o {
	case Yes:
		qYes += shares
	case No:
		qNo += shares
	}
	cost := cost(qYes, qNo, m.B) - m.Cost()
	return Quote{
		Outcome:     o,
		Shares:      shares,
		Cost:        cost,
		AvgPrice:    cost / shares,
		PriceBefore: before,
		PriceAfter:  priceAt(qYes, qNo, m.B, o),
	}, nil
}

// QuoteSell prices selling shares back to the market maker. The proceeds
// are C(q) - C(q - x): the market maker buys the shares back at the line it
// quotes after the sale, so selling into a falling price gets less per share.
func (m *Market) QuoteSell(o Outcome, shares float64) (Quote, error) {
	if err := m.checkTrade(o, shares); err != nil {
		return Quote{}, err
	}
	if shares > m.outstanding(o) {
		return Quote{}, fmt.Errorf("market: cannot sell %.4g %s shares, only %.4g outstanding", shares, o, m.outstanding(o))
	}
	before := m.mustPrice(o)
	qYes, qNo := m.QYes, m.QNo
	switch o {
	case Yes:
		qYes -= shares
	case No:
		qNo -= shares
	}
	proceeds := m.Cost() - cost(qYes, qNo, m.B)
	return Quote{
		Outcome:     o,
		Shares:      shares,
		Cost:        proceeds,
		AvgPrice:    proceeds / shares,
		PriceBefore: before,
		PriceAfter:  priceAt(qYes, qNo, m.B, o),
	}, nil
}

// Buy applies a buy: it quotes the trade, then moves the quantities. The
// caller handles payment, positions, and fees; this only moves the line.
func (m *Market) Buy(o Outcome, shares float64) (Quote, error) {
	q, err := m.QuoteBuy(o, shares)
	if err != nil {
		return Quote{}, err
	}
	switch o {
	case Yes:
		m.QYes += shares
	case No:
		m.QNo += shares
	}
	return q, nil
}

// Sell applies a sell back to the market maker and moves the line the
// other way.
func (m *Market) Sell(o Outcome, shares float64) (Quote, error) {
	q, err := m.QuoteSell(o, shares)
	if err != nil {
		return Quote{}, err
	}
	switch o {
	case Yes:
		m.QYes -= shares
	case No:
		m.QNo -= shares
	}
	return q, nil
}

// ImpactCurve shows how the line bends as a buy fills: the price of the
// outcome after each of steps equal slices of a shares buy. It quotes only,
// the market does not move.
func (m *Market) ImpactCurve(o Outcome, shares float64, steps int) ([]float64, error) {
	if err := m.checkTrade(o, shares); err != nil {
		return nil, err
	}
	if steps <= 0 {
		return nil, fmt.Errorf("market: steps must be positive, got %d", steps)
	}
	slice := shares / float64(steps)
	qYes, qNo := m.QYes, m.QNo
	curve := make([]float64, steps)
	for i := range curve {
		switch o {
		case Yes:
			qYes += slice
		case No:
			qNo += slice
		}
		curve[i] = priceAt(qYes, qNo, m.B, o)
	}
	return curve, nil
}

// MaxLoss is the market maker's worst-case loss: b * ln(2). No matter how
// one-sided the book gets, the maker can never lose more than this, because
// the most it ever pays out over what it collected is bounded by the cost
// function. Small b means a tight line but a small worst case; large b
// means a deep market that can lose more.
func (m *Market) MaxLoss() float64 {
	return m.B * math.Log(2)
}

// LossIf is what the market maker would lose if outcome o wins right now:
// the $1 payout on every outstanding share of that side, minus what traders
// have paid in so far. It is always at most MaxLoss.
func (m *Market) LossIf(o Outcome) (float64, error) {
	switch o {
	case Yes, No:
	default:
		return 0, fmt.Errorf("market: unknown outcome %q", o)
	}
	paidIn := m.Cost() - cost(0, 0, m.B)
	return m.outstanding(o) - paidIn, nil
}

func (m *Market) outstanding(o Outcome) float64 {
	if o == Yes {
		return m.QYes
	}
	return m.QNo
}

func (m *Market) checkTrade(o Outcome, shares float64) error {
	switch o {
	case Yes, No:
	default:
		return fmt.Errorf("market: unknown outcome %q", o)
	}
	if math.IsNaN(shares) || shares <= 0 {
		return fmt.Errorf("market: shares must be positive, got %.4g", shares)
	}
	return nil
}

func (m *Market) mustPrice(o Outcome) float64 {
	p, err := m.Price(o)
	if err != nil {
		panic(err) // unreachable: checkTrade already validated the outcome
	}
	return p
}

// priceAt is the LMSR price of an outcome at hypothetical quantities,
// without touching the market.
func priceAt(qYes, qNo, b float64, o Outcome) float64 {
	yes := 1 / (1 + math.Exp((qNo-qYes)/b))
	if o == Yes {
		return yes
	}
	return 1 - yes
}

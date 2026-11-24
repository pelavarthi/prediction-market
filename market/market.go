// Package market holds the core model for a binary prediction market:
// the market itself, its LMSR pricing, and the in-memory store.
package market

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Outcome is one side of a binary market.
type Outcome string

const (
	Yes Outcome = "YES"
	No  Outcome = "NO"
)

// Market is a single binary event market. Prices are set by a logarithmic
// market scoring rule (LMSR) market maker: it tracks how many YES and NO
// shares are outstanding (QYes, QNo) and a liquidity parameter b, and the
// price of each side is the softmax of the quantities.
type Market struct {
	ID        string
	Question  string
	B         float64 // liquidity: small b moves the line fast, large b moves it slowly
	QYes      float64 // YES shares outstanding
	QNo       float64 // NO shares outstanding
	Open      bool
	CreatedAt time.Time
}

// NewMarket validates its inputs and returns a market priced 50/50.
func NewMarket(id, question string, b float64) (*Market, error) {
	if question == "" {
		return nil, errors.New("market: question must not be empty")
	}
	if b <= 0 {
		return nil, errors.New("market: liquidity b must be positive")
	}
	return &Market{
		ID:        id,
		Question:  question,
		B:         b,
		Open:      true,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Price returns the current price of an outcome, between 0 and 1.
// It is the LMSR price: exp(q/b) over the sum of both sides, so the two
// prices always add up to 1.
func (m *Market) Price(o Outcome) (float64, error) {
	switch o {
	case Yes:
		return m.priceYes(), nil
	case No:
		return 1 - m.priceYes(), nil
	default:
		return 0, fmt.Errorf("market: unknown outcome %q", o)
	}
}

func (m *Market) priceYes() float64 {
	// 1 / (1 + exp((qNo - qYes) / b)): the stable form of the softmax,
	// so large quantities never overflow.
	return 1 / (1 + math.Exp((m.QNo-m.QYes)/m.B))
}

// Cost is the LMSR cost function C(q): what the market maker charges for
// the current quantities. Buying shares costs C(after) - C(before), which
// is how the line moves with order flow.
func (m *Market) Cost() float64 {
	return cost(m.QYes, m.QNo, m.B)
}

func cost(qYes, qNo, b float64) float64 {
	// b * ln(exp(qYes/b) + exp(qNo/b)), computed with the max trick so
	// large quantities don't overflow.
	top := math.Max(qYes, qNo)
	return top + b*math.Log(math.Exp((qYes-top)/b)+math.Exp((qNo-top)/b))
}

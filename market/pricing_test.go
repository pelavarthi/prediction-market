package market

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func mustMarket(b float64) *Market {
	m, err := NewMarket("m1", "Will it rain?", b)
	if err != nil {
		panic(err)
	}
	return m
}

func TestQuoteBuyCostMatchesCostFunction(t *testing.T) {
	m := mustMarket(100)
	q, err := m.QuoteBuy(Yes, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Buying 100 YES from (0, 0) costs C(100, 0) - C(0, 0).
	want := cost(100, 0, 100) - cost(0, 0, 100)
	if math.Abs(q.Cost-want) > 1e-9 {
		t.Errorf("buy 100 YES cost %.9f, want %.9f", q.Cost, want)
	}
	if q.PriceBefore != 0.5 {
		t.Errorf("price before = %.4f, want 0.5", q.PriceBefore)
	}
	// A buy pushes its own price up, so the average fill sits between
	// the price before and the price after the trade.
	if q.AvgPrice <= q.PriceBefore || q.AvgPrice >= q.PriceAfter {
		t.Errorf("avg price %.4f should sit between before %.4f and after %.4f",
			q.AvgPrice, q.PriceBefore, q.PriceAfter)
	}
	// Quoting must not move the market.
	if m.QYes != 0 || m.QNo != 0 {
		t.Errorf("quote moved the market: q=(%.1f, %.1f)", m.QYes, m.QNo)
	}
}

func TestBuyThenSellReturnsToStart(t *testing.T) {
	m := mustMarket(100)
	buy, err := m.Buy(Yes, 100)
	if err != nil {
		t.Fatal(err)
	}
	sell, err := m.Sell(Yes, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Selling back exactly what was bought refunds exactly what was paid:
	// the cost function only cares about the quantities, not the path.
	if math.Abs(buy.Cost-sell.Cost) > 1e-9 {
		t.Errorf("buy cost %.9f != sell proceeds %.9f", buy.Cost, sell.Cost)
	}
	if m.QYes != 0 {
		t.Errorf("quantities should be back to 0, got QYes %.9f", m.QYes)
	}
	yes, _ := m.Price(Yes)
	if yes != 0.5 {
		t.Errorf("price should be back to 0.5, got %.9f", yes)
	}
}

func TestBuyInTwoStepsCostsTheSameAsOne(t *testing.T) {
	one := mustMarket(100)
	q1, err := one.Buy(Yes, 100)
	if err != nil {
		t.Fatal(err)
	}

	two := mustMarket(100)
	a, err := two.Buy(Yes, 40)
	if err != nil {
		t.Fatal(err)
	}
	b, err := two.Buy(Yes, 60)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(q1.Cost-(a.Cost+b.Cost)) > 1e-9 {
		t.Errorf("buy 100 at once %.9f != buy 40 then 60 (%.9f + %.9f)",
			q1.Cost, a.Cost, b.Cost)
	}
}

func TestQuoteSellProceeds(t *testing.T) {
	m := mustMarket(100)
	if _, err := m.Buy(Yes, 100); err != nil {
		t.Fatal(err)
	}
	q, err := m.QuoteSell(Yes, 40)
	if err != nil {
		t.Fatal(err)
	}
	want := cost(100, 0, 100) - cost(60, 0, 100)
	if math.Abs(q.Cost-want) > 1e-9 {
		t.Errorf("sell 40 YES proceeds %.9f, want %.9f", q.Cost, want)
	}
	// Selling pushes the price down, so the average fill is below the
	// price before the sale.
	if q.AvgPrice >= q.PriceBefore || q.AvgPrice <= q.PriceAfter {
		t.Errorf("sell avg price %.4f should sit between after %.4f and before %.4f",
			q.AvgPrice, q.PriceAfter, q.PriceBefore)
	}
}

func TestTradesRejectBadInput(t *testing.T) {
	m := mustMarket(100)
	if _, err := m.Buy(Yes, 0); err == nil {
		t.Error("buying 0 shares should be rejected")
	}
	if _, err := m.Buy(Yes, -10); err == nil {
		t.Error("buying negative shares should be rejected")
	}
	if _, err := m.Buy("MAYBE", 10); err == nil {
		t.Error("unknown outcome should be rejected")
	}
	if _, err := m.Sell(Yes, 10); err == nil {
		t.Error("selling shares that were never bought should be rejected")
	}
	if _, err := m.Buy(Yes, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Sell(Yes, 11); err == nil {
		t.Error("selling more than outstanding should be rejected")
	}
	if _, err := m.ImpactCurve(Yes, 100, 0); err == nil {
		t.Error("impact curve with 0 steps should be rejected")
	}
}

func TestInvariantsHoldThroughRandomTrades(t *testing.T) {
	m := mustMarket(100)
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 500; i++ {
		o := Yes
		if rng.Intn(2) == 0 {
			o = No
		}
		shares := 1 + rng.Float64()*50
		if rng.Intn(2) == 0 || m.outstanding(o) == 0 {
			if _, err := m.Buy(o, shares); err != nil {
				t.Fatalf("trade %d: buy failed: %v", i, err)
			}
		} else {
			sell := math.Min(shares, m.outstanding(o))
			if _, err := m.Sell(o, sell); err != nil {
				t.Fatalf("trade %d: sell failed: %v", i, err)
			}
		}

		yes, _ := m.Price(Yes)
		no, _ := m.Price(No)
		if yes <= 0 || yes >= 1 || no <= 0 || no >= 1 {
			t.Fatalf("trade %d: prices out of (0,1): YES %.4f NO %.4f", i, yes, no)
		}
		if math.Abs(yes+no-1) > 1e-9 {
			t.Fatalf("trade %d: prices sum to %.9f, want 1", i, yes+no)
		}
		for _, o := range []Outcome{Yes, No} {
			loss, _ := m.LossIf(o)
			if loss > m.MaxLoss()+1e-6 {
				t.Fatalf("trade %d: market-maker loss if %s wins is %.4f, over the %.4f bound",
					i, o, loss, m.MaxLoss())
			}
		}
	}
}

func TestMarketMakerLossIsBoundedAndTight(t *testing.T) {
	m := mustMarket(100)
	if got, want := m.MaxLoss(), 100*math.Log(2); math.Abs(got-want) > 1e-9 {
		t.Errorf("MaxLoss = %.9f, want %.9f", got, want)
	}
	// One-sided buying pushes the loss toward the bound but never past it.
	for _, q := range []float64{100, 500, 1000, 5000} {
		mm := mustMarket(100)
		if _, err := mm.Buy(Yes, q); err != nil {
			t.Fatal(err)
		}
		loss, _ := mm.LossIf(Yes)
		if loss > mm.MaxLoss()+1e-9 {
			t.Errorf("q_yes=%.0f: loss %.4f exceeds bound %.4f", q, loss, mm.MaxLoss())
		}
	}
	// With a very one-sided book the loss is nearly the whole bound.
	mm := mustMarket(100)
	if _, err := mm.Buy(Yes, 5000); err != nil {
		t.Fatal(err)
	}
	loss, _ := mm.LossIf(Yes)
	if loss < 0.99*mm.MaxLoss() {
		t.Errorf("one-sided book loss %.4f should approach bound %.4f", loss, mm.MaxLoss())
	}
}

func TestLiquidityControlsHowFastTheLineMoves(t *testing.T) {
	// The same 100-share buy moves the line much further when b is small.
	impacts := map[float64]float64{}
	for _, b := range []float64{10, 100, 1000} {
		m := mustMarket(b)
		q, err := m.QuoteBuy(Yes, 100)
		if err != nil {
			t.Fatal(err)
		}
		impacts[b] = q.Impact()
	}
	if !(impacts[10] > impacts[100] && impacts[100] > impacts[1000]) {
		t.Errorf("price impact should shrink as b grows: b=10 %.4f, b=100 %.4f, b=1000 %.4f",
			impacts[10], impacts[100], impacts[1000])
	}
	// A deep market also charges closer to the starting price.
	for _, b := range []float64{10, 100, 1000} {
		m := mustMarket(b)
		q, _ := m.QuoteBuy(Yes, 100)
		if q.AvgPrice < 0.5 {
			t.Errorf("b=%.0f: buying YES from 50/50 should average above 0.5, got %.4f", b, q.AvgPrice)
		}
	}
}

func TestImpactCurve(t *testing.T) {
	m := mustMarket(100)
	curve, err := m.ImpactCurve(Yes, 100, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(curve) != 5 {
		t.Fatalf("curve has %d points, want 5", len(curve))
	}
	// Each slice pushes the price up, so the curve climbs.
	for i := 1; i < len(curve); i++ {
		if curve[i] <= curve[i-1] {
			t.Errorf("curve should climb: step %d %.4f <= step %d %.4f", i, curve[i], i-1, curve[i-1])
		}
	}
	// The last point is the price after the whole buy.
	q, _ := m.QuoteBuy(Yes, 100)
	if math.Abs(curve[len(curve)-1]-q.PriceAfter) > 1e-9 {
		t.Errorf("curve ends at %.9f, quote says %.9f", curve[len(curve)-1], q.PriceAfter)
	}
}

func ExampleMarket_QuoteBuy() {
	m, _ := NewMarket("m1", "Will it rain?", 100)
	for _, shares := range []float64{10, 50, 100} {
		q, _ := m.QuoteBuy(Yes, shares)
		fmt.Printf("buy %.0f YES: cost $%.2f, avg %.2f, line %.2f -> %.2f\n",
			shares, q.Cost, q.AvgPrice, q.PriceBefore, q.PriceAfter)
	}
	// Output:
	// buy 10 YES: cost $5.12, avg 0.51, line 0.50 -> 0.52
	// buy 50 YES: cost $28.09, avg 0.56, line 0.50 -> 0.62
	// buy 100 YES: cost $62.01, avg 0.62, line 0.50 -> 0.73
}

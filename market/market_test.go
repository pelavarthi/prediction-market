package market

import (
	"math"
	"testing"
)

func TestNewMarketRejectsBadInput(t *testing.T) {
	if _, err := NewMarket("", "", 100); err == nil {
		t.Error("empty question should be rejected")
	}
	if _, err := NewMarket("", "Will it rain?", 0); err == nil {
		t.Error("b = 0 should be rejected")
	}
	if _, err := NewMarket("", "Will it rain?", -10); err == nil {
		t.Error("negative b should be rejected")
	}
}

func TestPricesStartAtFiftyFifty(t *testing.T) {
	m, err := NewMarket("m1", "Will it rain?", 100)
	if err != nil {
		t.Fatal(err)
	}
	yes, _ := m.Price(Yes)
	no, _ := m.Price(No)
	if yes != 0.5 || no != 0.5 {
		t.Errorf("new market should be 50/50, got YES %.4f NO %.4f", yes, no)
	}
}

func TestPricesSumToOne(t *testing.T) {
	cases := []struct{ qYes, qNo float64 }{
		{0, 0}, {100, 0}, {0, 100}, {50, 200}, {1000, 10}, {37.5, 62.5},
	}
	for _, c := range cases {
		m, _ := NewMarket("m1", "Will it rain?", 100)
		m.QYes, m.QNo = c.qYes, c.qNo
		yes, _ := m.Price(Yes)
		no, _ := m.Price(No)
		if math.Abs(yes+no-1) > 1e-9 {
			t.Errorf("q=(%.1f, %.1f): prices sum to %.9f, want 1", c.qYes, c.qNo, yes+no)
		}
	}
}

func TestPricesStayInRange(t *testing.T) {
	m, _ := NewMarket("m1", "Will it rain?", 100)
	for _, q := range []struct{ qYes, qNo float64 }{{0, 0}, {1000, 0}, {0, 1000}, {500, 500}} {
		m.QYes, m.QNo = q.qYes, q.qNo
		yes, _ := m.Price(Yes)
		no, _ := m.Price(No)
		if yes <= 0 || yes >= 1 || no <= 0 || no >= 1 {
			t.Errorf("q=(%.1f, %.1f): prices out of (0,1): YES %.4f NO %.4f", q.qYes, q.qNo, yes, no)
		}
	}
	// At very large quantities float64 saturates to exactly 0 or 1, but the
	// price must never leave [0, 1].
	m.QYes, m.QNo = 10000, 0
	yes, _ := m.Price(Yes)
	if yes < 0 || yes > 1 {
		t.Errorf("extreme q: YES price %.4f out of [0,1]", yes)
	}
}

func TestPriceMovesWithQuantity(t *testing.T) {
	m, _ := NewMarket("m1", "Will it rain?", 100)
	before, _ := m.Price(Yes)
	m.QYes += 100 // more YES shares bought: YES should get pricier
	after, _ := m.Price(Yes)
	if after <= before {
		t.Errorf("buying YES should raise its price: %.4f -> %.4f", before, after)
	}
	m.QNo += 200 // now NO is the heavier side
	yes, _ := m.Price(Yes)
	if yes >= 0.5 {
		t.Errorf("heavier NO side should push YES below 0.5, got %.4f", yes)
	}
}

func TestPriceRejectsUnknownOutcome(t *testing.T) {
	m, _ := NewMarket("m1", "Will it rain?", 100)
	if _, err := m.Price("MAYBE"); err == nil {
		t.Error("unknown outcome should be rejected")
	}
}

func TestCostAtZero(t *testing.T) {
	m, _ := NewMarket("m1", "Will it rain?", 100)
	// C(0, 0) = b * ln(2): the market maker starts out owing nothing.
	if got, want := m.Cost(), 100*math.Log(2); math.Abs(got-want) > 1e-9 {
		t.Errorf("C(0,0) = %.9f, want %.9f", got, want)
	}
}

func TestCostGrowsWithQuantity(t *testing.T) {
	m, _ := NewMarket("m1", "Will it rain?", 100)
	before := m.Cost()
	m.QYes += 50
	if m.Cost() <= before {
		t.Error("cost should grow as shares are issued")
	}
}

func TestStoreCreateGetList(t *testing.T) {
	s := NewStore()
	a, _ := NewMarket("", "Will it rain?", 100)
	b, _ := NewMarket("", "Will UVA win?", 50)
	s.Create(a)
	s.Create(b)

	if a.ID == "" || b.ID == "" || a.ID == b.ID {
		t.Errorf("created markets need distinct IDs, got %q and %q", a.ID, b.ID)
	}
	got, ok := s.Get(a.ID)
	if !ok || got.Question != a.Question {
		t.Errorf("Get(%q) = %+v, %v", a.ID, got, ok)
	}
	if _, ok := s.Get("nope"); ok {
		t.Error("Get of a missing market should report false")
	}
	list := s.List()
	if len(list) != 2 || list[0] != a || list[1] != b {
		t.Errorf("List should return markets in creation order, got %d entries", len(list))
	}
}

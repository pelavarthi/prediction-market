package market

import (
	"math"
	"testing"
)

func TestCashOutCreditsBalanceAndShrinksPosition(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	// Push the YES price up so the cash-out sells at a profit.
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 40); err != nil {
		t.Fatal(err)
	}
	before, _ := m.Price(Yes)
	a, _ := l.Account("alice")
	balanceBefore := a.Balance
	qYesBefore := m.QYes

	co, err := l.CashOut("alice", m.ID, Yes, 10)
	if err != nil {
		t.Fatal(err)
	}
	if co.Shares != 10 || co.Outcome != Yes {
		t.Errorf("cash-out = %+v, want 10 YES shares", co)
	}
	if co.Fee != FeeRate*co.Proceeds {
		t.Errorf("fee = %.4f, want 1%% of proceeds %.4f", co.Fee, co.Proceeds)
	}
	if co.Net != co.Proceeds-co.Fee {
		t.Errorf("net = %.4f, want proceeds - fee = %.4f", co.Net, co.Proceeds-co.Fee)
	}
	if co.AvgPrice != co.Proceeds/10 {
		t.Errorf("avg price = %.4f, want proceeds/shares = %.4f", co.AvgPrice, co.Proceeds/10)
	}
	if co.RealizedPnL != co.Proceeds-co.CostBasis {
		t.Errorf("realized P&L = %.4f, want proceeds - basis = %.4f", co.RealizedPnL, co.Proceeds-co.CostBasis)
	}
	if co.RealizedPnL <= 0 {
		t.Errorf("selling into a higher price should profit, got P&L %.4f", co.RealizedPnL)
	}
	if co.CostBasis <= 0 {
		t.Error("cost basis should be what the shares were bought for")
	}

	// Selling YES moves the line back down.
	after, _ := m.Price(Yes)
	if after >= before {
		t.Errorf("selling YES should lower its price: %.4f -> %.4f", before, after)
	}
	if co.PriceAfter != after {
		t.Errorf("cash-out price after = %.4f, market price now %.4f", co.PriceAfter, after)
	}
	if m.QYes != qYesBefore-10 {
		t.Errorf("QYes = %.4f, want %.4f after selling 10", m.QYes, qYesBefore-10)
	}

	a, _ = l.Account("alice")
	if want := balanceBefore + co.Net; math.Abs(a.Balance-want) > 1e-9 {
		t.Errorf("balance = %.4f, want %.4f", a.Balance, want)
	}
	pos, _ := l.Position("alice", m.ID)
	if pos.Yes != 40 {
		t.Errorf("position YES = %.4f, want 40 after cashing out 10 of 50", pos.Yes)
	}
	if len(l.CashOuts("alice")) != 1 || l.CashOuts("alice")[0] != co {
		t.Error("cash-out should show up in the user's cash-out history")
	}
}

func TestCashOutPartialKeepsCostBasis(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	pos, _ := l.Position("alice", m.ID)
	basisBefore := pos.AvgCostYes

	if _, err := l.CashOut("alice", m.ID, Yes, 4); err != nil {
		t.Fatal(err)
	}
	pos, _ = l.Position("alice", m.ID)
	if pos.Yes != 6 {
		t.Errorf("position YES = %.4f, want 6 after a partial cash-out of 4", pos.Yes)
	}
	if pos.AvgCostYes != basisBefore {
		t.Errorf("avg cost = %.4f, want %.4f: the shares left behind keep their basis", pos.AvgCostYes, basisBefore)
	}

	// Cashing out the rest empties the position.
	if _, err := l.CashOut("alice", m.ID, Yes, 6); err != nil {
		t.Fatal(err)
	}
	pos, _ = l.Position("alice", m.ID)
	if pos.Yes != 0 {
		t.Errorf("position YES = %.4f, want 0 after cashing out everything", pos.Yes)
	}
	if len(l.CashOuts("alice")) != 2 {
		t.Errorf("want 2 cash-outs in history, got %d", len(l.CashOuts("alice")))
	}
}

func TestCashOutAtLoss(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	// Heavy NO buying crushes the YES price below what alice paid.
	if _, err := l.PlaceBuy("alice", m.ID, No, 100); err != nil {
		t.Fatal(err)
	}
	co, err := l.CashOut("alice", m.ID, Yes, 10)
	if err != nil {
		t.Fatal(err)
	}
	if co.RealizedPnL >= 0 {
		t.Errorf("selling into a lower price should lose money, got P&L %.4f", co.RealizedPnL)
	}
	if co.Net <= 0 {
		t.Errorf("net should still be positive cash back, got %.4f", co.Net)
	}
}

func TestCashOutRejectsMoreThanHeld(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	a, _ := l.Account("alice")
	balanceBefore := a.Balance
	qYesBefore := m.QYes

	if _, err := l.CashOut("alice", m.ID, Yes, 11); err == nil {
		t.Error("cashing out more shares than held should be rejected")
	}
	a, _ = l.Account("alice")
	if a.Balance != balanceBefore {
		t.Errorf("failed cash-out changed the balance to %.4f", a.Balance)
	}
	if m.QYes != qYesBefore {
		t.Error("failed cash-out moved the line")
	}
	pos, _ := l.Position("alice", m.ID)
	if pos.Yes != 10 {
		t.Errorf("failed cash-out changed the position to %.4f", pos.Yes)
	}
	if len(l.CashOuts("alice")) != 0 {
		t.Error("failed cash-out was recorded")
	}
}

func TestCashOutRejectsNoPosition(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.CashOut("alice", m.ID, Yes, 1); err == nil {
		t.Error("cashing out with no position should be rejected")
	}
	if _, err := l.CashOut("alice", m.ID, No, 1); err == nil {
		t.Error("cashing out the other side with no position should be rejected")
	}
}

func TestCashOutRejectsClosedMarket(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	m.Open = false
	if _, err := l.CashOut("alice", m.ID, Yes, 5); err == nil {
		t.Error("cash-out on a closed market should be rejected")
	}
}

func TestCashOutRejectsBadInput(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		user   string
		market string
		o      Outcome
		shares float64
	}{
		{"mallory", m.ID, Yes, 5},
		{"alice", "mkt-nope", Yes, 5},
		{"alice", m.ID, "MAYBE", 5},
		{"alice", m.ID, Yes, 0},
		{"alice", m.ID, Yes, -5},
	} {
		if _, err := l.CashOut(tc.user, tc.market, tc.o, tc.shares); err == nil {
			t.Errorf("cash-out %+v should be rejected", tc)
		}
	}
}

func TestQuoteCashOutDoesNotMoveAnything(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 40); err != nil {
		t.Fatal(err)
	}
	a, _ := l.Account("alice")
	balanceBefore := a.Balance
	qYesBefore := m.QYes

	q, err := l.QuoteCashOut("alice", m.ID, Yes, 10)
	if err != nil {
		t.Fatal(err)
	}
	if q.Fee != FeeRate*q.Proceeds {
		t.Errorf("quote fee = %.4f, want 1%% of proceeds %.4f", q.Fee, q.Proceeds)
	}
	if q.Net != q.Proceeds-q.Fee {
		t.Errorf("quote net = %.4f, want proceeds - fee", q.Net)
	}
	if m.QYes != qYesBefore {
		t.Error("a quote should not move the line")
	}
	a, _ = l.Account("alice")
	if a.Balance != balanceBefore {
		t.Error("a quote should not touch the balance")
	}
	pos, _ := l.Position("alice", m.ID)
	if pos.Yes != 50 {
		t.Errorf("a quote should not touch the position, got %.4f YES", pos.Yes)
	}

	// The real cash-out matches the quote.
	co, err := l.CashOut("alice", m.ID, Yes, 10)
	if err != nil {
		t.Fatal(err)
	}
	if co.Proceeds != q.Proceeds || co.Fee != q.Fee || co.Net != q.Net {
		t.Errorf("cash-out %+v does not match its quote %+v", co, q)
	}
	if co.PriceAfter != q.PriceAfter {
		t.Errorf("cash-out price after %.4f, quote said %.4f", co.PriceAfter, q.PriceAfter)
	}
}

func TestQuoteCashOutRejectsMoreThanHeld(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := l.QuoteCashOut("alice", m.ID, Yes, 11); err == nil {
		t.Error("quoting more shares than held should be rejected")
	}
	if _, err := l.QuoteCashOut("alice", m.ID, No, 1); err == nil {
		t.Error("quoting a side with no position should be rejected")
	}
}

func TestCashOutLedgerEntries(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	co, err := l.CashOut("alice", m.ID, Yes, 4)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := l.Account("alice")
	sum := 0.0
	cashouts := 0.0
	fees := 0.0
	for _, e := range a.Ledger {
		sum += e.Amount
		if e.Kind == "cashout" {
			cashouts += e.Amount
		}
		if e.Kind == "fee" {
			fees += -e.Amount
		}
	}
	if math.Abs(sum-a.Balance) > 1e-9 {
		t.Errorf("ledger sums to %.4f but balance is %.4f", sum, a.Balance)
	}
	if cashouts != co.Proceeds {
		t.Errorf("ledger cash-out entries sum to %.4f, want proceeds %.4f", cashouts, co.Proceeds)
	}
	if fees <= 0 {
		t.Error("ledger should show the 1% cash-out fee as its own entry")
	}
}

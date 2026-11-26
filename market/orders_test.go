package market

import (
	"math"
	"testing"
)

// setupLedger builds a ledger with one open market (b = 100, 50/50) and one
// user holding $1,000.
func setupLedger(t *testing.T) (*Ledger, *Market) {
	t.Helper()
	store := NewStore()
	m, err := NewMarket("", "Will it rain?", 100)
	if err != nil {
		t.Fatal(err)
	}
	store.Create(m)
	l := NewLedger(store)
	if _, err := l.CreateAccount("alice", 1000); err != nil {
		t.Fatal(err)
	}
	return l, m
}

func TestPlaceBuyCreditsPositionAndDebitsBalance(t *testing.T) {
	l, m := setupLedger(t)
	before, _ := m.Price(Yes)

	order, err := l.PlaceBuy("alice", m.ID, Yes, 10)
	if err != nil {
		t.Fatal(err)
	}
	if order.Shares != 10 || order.Outcome != Yes {
		t.Errorf("order = %+v, want 10 YES shares", order)
	}
	if order.Fee != FeeRate*order.Cost {
		t.Errorf("fee = %.4f, want 1%% of cost %.4f", order.Fee, order.Cost)
	}
	if order.Total != order.Cost+order.Fee {
		t.Errorf("total = %.4f, want cost + fee = %.4f", order.Total, order.Cost+order.Fee)
	}
	if order.AvgPrice != order.Cost/10 {
		t.Errorf("avg price = %.4f, want cost/shares = %.4f", order.AvgPrice, order.Cost/10)
	}

	a, _ := l.Account("alice")
	if want := 1000 - order.Total; math.Abs(a.Balance-want) > 1e-9 {
		t.Errorf("balance = %.4f, want %.4f", a.Balance, want)
	}
	pos, ok := l.Position("alice", m.ID)
	if !ok || pos.Yes != 10 || pos.No != 0 {
		t.Errorf("position = %+v, %v, want 10 YES", pos, ok)
	}
	after, _ := m.Price(Yes)
	if after <= before {
		t.Errorf("buying YES should raise its price: %.4f -> %.4f", before, after)
	}
	if order.PriceAfter != after {
		t.Errorf("order price after = %.4f, market price now %.4f", order.PriceAfter, after)
	}
	if len(l.Orders("alice")) != 1 || l.Orders("alice")[0] != order {
		t.Error("filled order should show up in the user's order history")
	}
}

func TestPlaceBuyNoSide(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, No, 5); err != nil {
		t.Fatal(err)
	}
	pos, _ := l.Position("alice", m.ID)
	if pos.No != 5 || pos.Yes != 0 {
		t.Errorf("position = %+v, want 5 NO", pos)
	}
	yes, _ := m.Price(Yes)
	if yes >= 0.5 {
		t.Errorf("buying NO should push YES below 0.5, got %.4f", yes)
	}
}

func TestPlaceBuyAccumulatesPosition(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 4); err != nil {
		t.Fatal(err)
	}
	pos, _ := l.Position("alice", m.ID)
	if pos.Yes != 14 {
		t.Errorf("position YES = %.4f, want 14 after two buys", pos.Yes)
	}
	if len(l.Orders("alice")) != 2 {
		t.Errorf("want 2 orders in history, got %d", len(l.Orders("alice")))
	}
}

func TestPlaceBuyRejectsInsufficientFunds(t *testing.T) {
	l, m := setupLedger(t)
	qYesBefore := m.QYes

	// 10,000 YES shares at ~50c costs far more than the $1,000 balance.
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10000); err == nil {
		t.Error("buy beyond the balance should be rejected")
	}
	a, _ := l.Account("alice")
	if a.Balance != 1000 {
		t.Errorf("failed buy changed the balance to %.4f", a.Balance)
	}
	if m.QYes != qYesBefore {
		t.Error("failed buy moved the line")
	}
	if _, ok := l.Position("alice", m.ID); ok {
		t.Error("failed buy created a position")
	}
	if len(l.Orders("alice")) != 0 {
		t.Error("failed buy was recorded as an order")
	}
}

func TestPlaceBuyRejectsClosedMarket(t *testing.T) {
	l, m := setupLedger(t)
	m.Open = false
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err == nil {
		t.Error("buy on a closed market should be rejected")
	}
}

func TestPlaceBuyRejectsUnknownUserAndMarket(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("mallory", m.ID, Yes, 10); err == nil {
		t.Error("buy for an unknown user should be rejected")
	}
	if _, err := l.PlaceBuy("alice", "mkt-nope", Yes, 10); err == nil {
		t.Error("buy on an unknown market should be rejected")
	}
}

func TestPlaceBuyRejectsBadOutcomeAndShares(t *testing.T) {
	l, m := setupLedger(t)
	for _, tc := range []struct {
		o      Outcome
		shares float64
	}{
		{"MAYBE", 10},
		{Yes, 0},
		{Yes, -5},
	} {
		if _, err := l.PlaceBuy("alice", m.ID, tc.o, tc.shares); err == nil {
			t.Errorf("buy %v %.4g should be rejected", tc.o, tc.shares)
		}
	}
}

func TestLedgerSumsToBalance(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := l.PlaceBuy("alice", m.ID, No, 5); err != nil {
		t.Fatal(err)
	}
	if err := l.Deposit("alice", 50); err != nil {
		t.Fatal(err)
	}
	a, _ := l.Account("alice")
	sum := 0.0
	for _, e := range a.Ledger {
		sum += e.Amount
		if e.BalanceAfter < 0 {
			t.Errorf("ledger entry %+v leaves a negative balance", e)
		}
	}
	if math.Abs(sum-a.Balance) > 1e-9 {
		t.Errorf("ledger sums to %.4f but balance is %.4f", sum, a.Balance)
	}
	// Opening deposit + 2 buys (cost + fee each) + 1 deposit.
	if len(a.Ledger) != 6 {
		t.Errorf("want 6 ledger entries, got %d", len(a.Ledger))
	}
	fees := 0.0
	for _, e := range a.Ledger {
		if e.Kind == "fee" {
			fees += -e.Amount
		}
	}
	if fees <= 0 {
		t.Error("ledger should show the 1% fees as their own entries")
	}
}

func TestCreateAccountValidation(t *testing.T) {
	l, _ := setupLedger(t)
	if _, err := l.CreateAccount("", 100); err == nil {
		t.Error("empty user id should be rejected")
	}
	if _, err := l.CreateAccount("bob", -1); err == nil {
		t.Error("negative starting balance should be rejected")
	}
	if _, err := l.CreateAccount("alice", 100); err == nil {
		t.Error("duplicate account should be rejected")
	}
	if _, ok := l.Account("nobody"); ok {
		t.Error("unknown user should report false")
	}
}

func TestDepositValidation(t *testing.T) {
	l, _ := setupLedger(t)
	if err := l.Deposit("alice", 0); err == nil {
		t.Error("zero deposit should be rejected")
	}
	if err := l.Deposit("alice", -10); err == nil {
		t.Error("negative deposit should be rejected")
	}
	if err := l.Deposit("nobody", 10); err == nil {
		t.Error("deposit for an unknown user should be rejected")
	}
	if err := l.Deposit("alice", 25); err != nil {
		t.Fatal(err)
	}
	a, _ := l.Account("alice")
	if a.Balance != 1025 {
		t.Errorf("balance = %.4f, want 1025 after deposit", a.Balance)
	}
}

func TestPositionSharesHelper(t *testing.T) {
	l, m := setupLedger(t)
	if _, err := l.PlaceBuy("alice", m.ID, Yes, 7); err != nil {
		t.Fatal(err)
	}
	pos, _ := l.Position("alice", m.ID)
	if pos.Shares(Yes) != 7 || pos.Shares(No) != 0 {
		t.Errorf("Shares() = YES %.4f NO %.4f, want 7 and 0", pos.Shares(Yes), pos.Shares(No))
	}
}

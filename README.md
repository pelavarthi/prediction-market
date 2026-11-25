# prediction-market

Kalshi-style prediction market in Go: binary event markets priced by an
LMSR market maker, live cash-out, live prices, and settlement.

A market asks a yes/no question ("Will it rain in Charlottesville on
Saturday?"). Each side has a price between 0 and 1, the market's implied
probability. Buying YES at 62c pays $1 if the event happens. The line is set
by a logarithmic market scoring rule (LMSR) market maker: it holds a quantity
of YES and NO shares outstanding and a liquidity parameter `b`, and the price
moves smoothly as money comes in. YES and NO always sum to $1, and the market
maker's worst-case loss is bounded at `b * ln(2)`.

## Layout

- `market/` — the market model, LMSR pricing, and the in-memory store
- `cmd/server/` — the HTTP server

## Run it

```sh
go run ./cmd/server
```

Then `curl localhost:8080/health` should return `{"status":"ok"}`.

## Pricing

The line is set by the market maker, not by matching buyers and sellers.
Buying shares costs `C(q + x) - C(q)`, where `C` is the LMSR cost function,
so a big order pays a worse average price than a small one: the line moves
while the trade fills. `Market.QuoteBuy` / `QuoteSell` price a trade without
moving the market; `Buy` / `Sell` apply it. `ImpactCurve` shows how the
price bends as a buy fills, and `LossIf` / `MaxLoss` show the market
maker's exposure: its worst-case loss is always at most `b * ln(2)`. Small
`b` means a twitchy line, large `b` means a deep market.

## Test it

```sh
go test ./...
```

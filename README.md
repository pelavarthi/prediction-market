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

## Test it

```sh
go test ./...
```

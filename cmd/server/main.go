// Command server runs the prediction market HTTP server.
package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/pelavarthi/prediction-market/market"
)

func main() {
	store := market.NewStore()
	seed(store)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	log.Println("prediction market listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// seed loads one example market so the server has something to price.
func seed(s *market.Store) {
	m, err := market.NewMarket("", "Will it rain in Charlottesville on Saturday?", 100)
	if err != nil {
		log.Fatalf("seed market: %v", err)
	}
	s.Create(m)
	log.Printf("seeded market %s: %s", m.ID, m.Question)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

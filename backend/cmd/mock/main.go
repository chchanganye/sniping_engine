package main

import (
	crand "crypto/rand"
	"encoding/json"
	"flag"
	"log"
	"math/rand"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	mux := http.NewServeMux()
	mux.HandleFunc("/mock/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	mux.HandleFunc("/mock/login-by-sms", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"token":    "mock_token_" + randString(8),
				"deviceId": "mock_device_" + randString(8),
				"uuid":     randString(12),
			},
		})
	})

	mux.HandleFunc("/mock/preflight-order", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		// 50% canBuy (simulated)
		canBuy := rand.Intn(2) == 0
		totalFee := int64(1800)
		if qty, ok := body["quantity"].(float64); ok && qty > 0 {
			totalFee = int64(qty) * 1800
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"canBuy":    canBuy,
				"totalFee":  totalFee,
				"traceId":   randString(10),
				"timestamp": time.Now().UnixMilli(),
			},
		})
	})

	mux.HandleFunc("/mock/create-order", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		success := false
		if v, ok := body["totalFee"].(float64); ok && v > 0 {
			success = true
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": success,
			"data": map[string]any{
				"orderId":    rand.Int63n(900000000000) + 100000000000,
				"createdAt":  time.Now().Format(time.RFC3339Nano),
				"purchaseId": rand.Int63n(900000000000) + 100000000000,
			},
		})
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("mock listening on %s", *addr)
	log.Fatal(srv.ListenAndServe())
}

func randString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	if n <= 0 {
		return ""
	}
	raw := make([]byte, n)
	_, _ = crand.Read(raw)
	out := make([]byte, n)
	for i := range out {
		out[i] = letters[int(raw[i])%len(letters)]
	}
	return string(out)
}

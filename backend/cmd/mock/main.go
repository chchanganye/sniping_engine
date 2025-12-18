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

	mux.HandleFunc("/mock/api/user/web/shipping-address/self/list-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": []map[string]any{
				{
					"id":              34507417,
					"receiveUserName": "张三",
					"mobile":          "176****3830",
					"province":        "上海",
					"city":            "上海市",
					"region":          "浦东新区",
					"street":          "川沙新镇",
					"detail":          "川沙新镇黄赵路310号(迪士尼地铁站1号口步行300米)",
					"isDefault":       true,
					"longitude":       121.667003,
					"latitude":        31.141447,
				},
			},
		})
	})

	mux.HandleFunc("/mock/api/item/shop-category/tree", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": []map[string]any{
				{
					"id":          1001,
					"pid":         0,
					"level":       1,
					"name":        "Mock 一级分类",
					"hasChildren": true,
					"childrenList": []map[string]any{
						{
							"id":           2001,
							"pid":          1001,
							"level":        2,
							"name":         "Mock 二级分类 A",
							"hasChildren":  false,
							"childrenList": []map[string]any{},
						},
					},
				},
			},
		})
	})

	mux.HandleFunc("/mock/api/item/store/item/searchStoreSkuByCategory", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": []map[string]any{
				{
					"categoryId":   1514,
					"categoryName": "Mock 商品分组",
					"logo":         nil,
					"storeSkuModelList": []map[string]any{
						{
							"id":              110005201029005,
							"skuId":           110005201029005,
							"itemId":          110005201029005,
							"storeId":         1100078037,
							"shopId":          1100078037,
							"categoryId":      1514,
							"itemCode":        "goods1310016",
							"fullUnit":        "个",
							"name":            "招财纳福牌",
							"mainImage":       "https://assets.4008117117.com/upload/2025/1/7/b9c2f7d3-b787-4c0d-9132-a0cc3e719bba.jpg",
							"price":           1800,
							"originalPrice":   1800,
							"inStock":         10,
							"purchaseLimit":   2,
							"maxPurchaseLimit": 2,
							"riskFlag":        nil,
						},
						{
							"id":              110005201028004,
							"skuId":           110005201028004,
							"itemId":          110005201028004,
							"storeId":         1100078037,
							"shopId":          1100078037,
							"categoryId":      1514,
							"itemCode":        "goods1311032",
							"fullUnit":        "个",
							"name":            "瑞蛇起舞扣",
							"mainImage":       "https://assets.4008117117.com/upload/2025/1/4/09377989-b609-40b6-8580-d8dbe0363c91.jpg",
							"price":           2800,
							"originalPrice":   2800,
							"inStock":         5,
							"purchaseLimit":   1,
							"maxPurchaseLimit": 1,
							"riskFlag":        nil,
						},
					},
				},
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

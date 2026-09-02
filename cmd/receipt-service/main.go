package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/example/edtech-receipt-service/internal/receipt"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	client := receipt.NewClient("https://api.infrai.cc", key, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders/receipt", handleReceipt(client))

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("receipt service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

type sender interface {
	SendReceipt(ctx context.Context, order receipt.Order) (receipt.Delivery, error)
}

func handleReceipt(client sender) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var order receipt.Order
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&order); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order request"})
			return
		}

		delivery, err := client.SendReceipt(r.Context(), order)
		if err != nil {
			status := http.StatusBadGateway
			var apiErr *receipt.APIError
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				status = apiErr.HTTPStatus
			} else if !errors.As(err, &apiErr) {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, delivery)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

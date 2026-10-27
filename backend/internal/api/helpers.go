package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"minimarket/internal/store"
)

func decodeBody(r *http.Request, destination any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON value")
	}
	return nil
}

func writeFailure(w http.ResponseWriter, err error) {
	var typed apiError
	if errors.As(err, &typed) {
		writeJSON(w, typed.status, map[string]string{"detail": typed.detail})
		return
	}
	log.Printf("handle API request: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error interno del servidor."})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode API response: %v", err)
	}
}

func validPositivePrice(price float64) bool {
	return price > 0 && !math.IsNaN(price) && !math.IsInf(price, 0)
}

func money(value float64) float64 {
	return math.Round(value*100) / 100
}

func parseExpiry(value string, days int) (string, error) {
	if value == "" {
		return utcToday().AddDate(0, 0, days).Format("2006-01-02") + "T00:00:00Z", nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", err
	}
	return date.Format("2006-01-02") + "T00:00:00Z", nil
}

func parseDay(value string) (time.Time, error) {
	if len(value) >= 10 {
		if date, err := time.Parse("2006-01-02", value[:10]); err == nil {
			return date, nil
		}
	}
	date, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	year, month, day := date.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

func daysUntil(value string) (int, error) {
	expiry, err := parseDay(value)
	if err != nil {
		return 0, err
	}
	return int(expiry.Sub(utcToday()).Hours() / 24), nil
}

func utcToday() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func findProductByID(data *store.Data, id int) *store.Product {
	for i := range data.Products {
		if data.Products[i].ID == id {
			return &data.Products[i]
		}
	}
	return nil
}

func findProductByBarcode(data *store.Data, barcode string) *store.Product {
	for i := range data.Products {
		if data.Products[i].Barcode == barcode {
			return &data.Products[i]
		}
	}
	return nil
}

func findLotByID(data *store.Data, id int) *store.Lot {
	for i := range data.Lots {
		if data.Lots[i].ID == id {
			return &data.Lots[i]
		}
	}
	return nil
}

func firstAvailableLot(data *store.Data, productID int) *store.Lot {
	var selected *store.Lot
	for i := range data.Lots {
		lot := &data.Lots[i]
		if lot.ProductID != productID || lot.Available <= 0 {
			continue
		}
		if selected == nil || lot.ExpiresAt < selected.ExpiresAt || (lot.ExpiresAt == selected.ExpiresAt && lot.ReceivedAt < selected.ReceivedAt) {
			selected = lot
		}
	}
	return selected
}

func formatMinimarketError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w", err)
}

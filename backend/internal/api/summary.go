package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type dailySalesSummary struct {
	Date      string  `json:"fecha"`
	SaleCount int     `json:"cantidad_ventas"`
	TotalPaid float64 `json:"total_ventas"`
}

type dashboardSummary struct {
	TotalProducts    int                 `json:"productos_total"`
	ExpiredProducts  int                 `json:"productos_vencidos"`
	ExpiredUnits     int                 `json:"unidades_vencidas"`
	ExpiringProducts int                 `json:"productos_por_vencer"`
	ExpiringUnits    int                 `json:"unidades_por_vencer"`
	TodaySaleCount   int                 `json:"ventas_hoy"`
	TodaySalesTotal  float64             `json:"total_ventas_hoy"`
	SalesPerDay      []dailySalesSummary `json:"ventas_por_dia"`
}

func (h *Handler) getDashboardSummary(w http.ResponseWriter) {
	data, err := h.repository.Read()
	if err != nil {
		writeFailure(w, err)
		return
	}

	today := utcToday()
	start := today.AddDate(0, 0, -6)
	expiringLimit := today.AddDate(0, 0, 7)
	expiredProducts := make(map[int]struct{})
	expiringProducts := make(map[int]struct{})
	summary := dashboardSummary{
		TotalProducts: len(data.Products),
		SalesPerDay:   make([]dailySalesSummary, 7),
	}
	dailySales := make(map[string]int, len(summary.SalesPerDay))
	for i := range summary.SalesPerDay {
		date := start.AddDate(0, 0, i)
		key := date.Format("2006-01-02")
		summary.SalesPerDay[i].Date = key
		dailySales[key] = i
	}

	for _, lot := range data.Lots {
		if lot.Available <= 0 || findProductByID(&data, lot.ProductID) == nil {
			continue
		}
		expiry, err := parseDay(lot.ExpiresAt)
		if err != nil {
			writeFailure(w, fmt.Errorf("read expiry for lot %d: %w", lot.ID, err))
			return
		}
		switch {
		case expiry.Before(today):
			expiredProducts[lot.ProductID] = struct{}{}
			summary.ExpiredUnits += lot.Available
		case !expiry.After(expiringLimit):
			expiringProducts[lot.ProductID] = struct{}{}
			summary.ExpiringUnits += lot.Available
		}
	}
	summary.ExpiredProducts = len(expiredProducts)
	summary.ExpiringProducts = len(expiringProducts)

	for _, sale := range data.Sales {
		date, err := parseDay(sale.SoldAt)
		if err != nil {
			writeFailure(w, fmt.Errorf("read sale date for sale %d: %w", sale.ID, err))
			return
		}
		index, exists := dailySales[date.Format("2006-01-02")]
		if !exists {
			continue
		}
		summary.SalesPerDay[index].SaleCount++
		summary.SalesPerDay[index].TotalPaid = money(summary.SalesPerDay[index].TotalPaid + sale.TotalPaid)
		if date.Equal(today) {
			summary.TodaySaleCount++
			summary.TodaySalesTotal = money(summary.TodaySalesTotal + sale.TotalPaid)
		}
	}

	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) listExpiringLots(w http.ResponseWriter, r *http.Request) {
	days := 7
	if raw := r.URL.Query().Get("dias"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed > 90 {
			writeFailure(w, apiError{http.StatusBadRequest, "El parámetro dias debe estar entre 0 y 90."})
			return
		}
		days = parsed
	}
	data, err := h.repository.Read()
	if err != nil {
		writeFailure(w, err)
		return
	}
	today := utcToday()
	limit := today.AddDate(0, 0, days)
	result := make([]map[string]any, 0)
	for _, lot := range data.Lots {
		if lot.Available <= 0 {
			continue
		}
		expiry, err := parseDay(lot.ExpiresAt)
		if err != nil {
			writeFailure(w, fmt.Errorf("read expiry for lot %d: %w", lot.ID, err))
			return
		}
		if expiry.After(limit) {
			continue
		}
		product := findProductByID(&data, lot.ProductID)
		if product == nil {
			continue
		}
		result = append(result, map[string]any{
			"lote_id": lot.ID, "producto_id": product.ID, "codigo_barras": product.Barcode,
			"nombre_producto": product.Name, "categoria": product.Category, "numero_lote": lot.Number,
			"cantidad_disponible": lot.Available, "fecha_vencimiento": lot.ExpiresAt,
			"dias_para_vencer": int(expiry.Sub(today).Hours() / 24),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		expiryI := result[i]["fecha_vencimiento"].(string)
		expiryJ := result[j]["fecha_vencimiento"].(string)
		if expiryI == expiryJ {
			return strings.ToLower(result[i]["nombre_producto"].(string)) < strings.ToLower(result[j]["nombre_producto"].(string))
		}
		return expiryI < expiryJ
	})
	writeJSON(w, http.StatusOK, result)
}

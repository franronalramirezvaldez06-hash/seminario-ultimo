package api

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"minimarket/internal/store"
)

func (h *Handler) listInventory(w http.ResponseWriter) {
	data, err := h.repository.Read()
	if err != nil {
		writeFailure(w, err)
		return
	}
	result := make([]map[string]any, 0, len(data.Products))
	for _, product := range data.Products {
		stock, activeLots := 0, 0
		var nextExpiry *string
		for _, lot := range data.Lots {
			if lot.ProductID != product.ID || lot.Available <= 0 {
				continue
			}
			stock += lot.Available
			activeLots++
			if nextExpiry == nil || lot.ExpiresAt < *nextExpiry {
				expiry := lot.ExpiresAt
				nextExpiry = &expiry
			}
		}
		result = append(result, map[string]any{
			"producto_id": product.ID, "codigo_barras": product.Barcode, "nombre": product.Name,
			"categoria": product.Category, "precio_regular": product.RegularPrice,
			"stock_disponible": stock, "lotes_activos": activeLots, "proximo_vencimiento": nextExpiry,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]["nombre"].(string)) < strings.ToLower(result[j]["nombre"].(string))
	})
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) receiveLot(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProductID    int    `json:"producto_id"`
		Quantity     int    `json:"cantidad"`
		DaysToExpiry int    `json:"dias_para_vencer"`
		ExpiryDate   string `json:"fecha_vencimiento"`
	}
	if err := decodeBody(r, &input); err != nil {
		writeFailure(w, apiError{http.StatusBadRequest, "Solicitud JSON inválida."})
		return
	}
	if input.ProductID < 1 || input.Quantity < 1 || input.DaysToExpiry < 0 {
		writeFailure(w, apiError{http.StatusBadRequest, "Producto, cantidad y días para vencer deben ser válidos."})
		return
	}
	expires, err := parseExpiry(input.ExpiryDate, input.DaysToExpiry)
	if err != nil {
		writeFailure(w, apiError{http.StatusBadRequest, "La fecha de vencimiento no es válida."})
		return
	}

	result, err := h.repository.Update(func(data *store.Data) (any, error) {
		if findProductByID(data, input.ProductID) == nil {
			return nil, apiError{http.StatusNotFound, "Producto no encontrado."}
		}
		id := data.NextIDs["lote"]
		if id < 1 {
			id = 1
		}
		data.NextIDs["lote"] = id + 1
		now := time.Now().UTC()
		lot := store.Lot{
			ID: id, ProductID: input.ProductID,
			Number:    fmt.Sprintf("LOT-%d-%s", input.ProductID, now.Format("20060102150405.000000000")),
			Available: input.Quantity, ExpiresAt: expires, ReceivedAt: now.Format(time.RFC3339Nano),
		}
		data.Lots = append(data.Lots, lot)
		return map[string]any{
			"mensaje": "Lote ingresado correctamente", "lote_id": id, "producto_id": input.ProductID,
			"cantidad_disponible": input.Quantity, "fecha_vencimiento": expires,
		}, nil
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) getFEFOLot(w http.ResponseWriter, r *http.Request) {
	barcode, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/lotes/peps/"))
	if err != nil || barcode == "" {
		writeFailure(w, apiError{http.StatusBadRequest, "Código de barras inválido."})
		return
	}
	data, err := h.repository.Read()
	if err != nil {
		writeFailure(w, err)
		return
	}
	product := findProductByBarcode(&data, barcode)
	if product == nil {
		writeFailure(w, apiError{http.StatusNotFound, "Producto no encontrado."})
		return
	}
	lot := firstAvailableLot(&data, product.ID)
	if lot == nil {
		writeFailure(w, apiError{http.StatusNotFound, "No existe un lote activo para ese producto."})
		return
	}
	days, err := daysUntil(lot.ExpiresAt)
	if err != nil {
		writeFailure(w, fmt.Errorf("read expiry for lot %d: %w", lot.ID, err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": lot.ID, "producto_id": lot.ProductID, "numero_lote": lot.Number,
		"cantidad_disponible": lot.Available, "fecha_vencimiento": lot.ExpiresAt,
		"fecha_ingreso": lot.ReceivedAt, "codigo_barras": product.Barcode, "nombre": product.Name,
		"categoria": product.Category, "precio_regular": product.RegularPrice,
		"dias_hasta_vencimiento": days, "descuento_aplicado": days <= 5,
	})
}

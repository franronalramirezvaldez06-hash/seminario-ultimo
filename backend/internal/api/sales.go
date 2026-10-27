package api

import (
	"fmt"
	"net/http"
	"time"

	"minimarket/internal/store"
)

func (h *Handler) createSale(w http.ResponseWriter, r *http.Request) {
	var input []struct {
		Barcode  string `json:"codigo_barras"`
		Quantity int    `json:"cantidad"`
	}
	if err := decodeBody(r, &input); err != nil || len(input) == 0 {
		writeFailure(w, apiError{http.StatusBadRequest, "La venta debe incluir al menos un producto válido."})
		return
	}
	quantities := make(map[string]int, len(input))
	for _, item := range input {
		if item.Barcode == "" || item.Quantity < 1 {
			writeFailure(w, apiError{http.StatusBadRequest, "Todas las cantidades deben ser números enteros mayores que cero."})
			return
		}
		quantities[item.Barcode] += item.Quantity
	}

	result, err := h.repository.Update(func(data *store.Data) (any, error) {
		type saleItem struct {
			product  *store.Product
			lot      *store.Lot
			quantity int
		}
		plan := make([]saleItem, 0, len(quantities))
		for barcode, quantity := range quantities {
			product := findProductByBarcode(data, barcode)
			if product == nil {
				return nil, apiError{http.StatusNotFound, "Producto no encontrado para código " + barcode + "."}
			}
			lot := firstAvailableLot(data, product.ID)
			if lot == nil {
				return nil, apiError{http.StatusNotFound, "No existe un lote activo para " + product.Name + "."}
			}
			if quantity > lot.Available {
				return nil, apiError{http.StatusBadRequest, "La cantidad pedida excede el stock disponible del lote FEFO para " + product.Name + "."}
			}
			plan = append(plan, saleItem{product, lot, quantity})
		}

		saleID := data.NextIDs["venta"]
		if saleID < 1 {
			saleID = 1
		}
		data.NextIDs["venta"] = saleID + 1
		sale := store.Sale{ID: saleID, SoldAt: time.Now().UTC().Format(time.RFC3339Nano), Details: []store.SaleDetail{}}
		for _, item := range plan {
			days, err := daysUntil(item.lot.ExpiresAt)
			if err != nil {
				return nil, fmt.Errorf("read expiry for lot %d: %w", item.lot.ID, err)
			}
			discounted := days <= 5
			unitPrice := item.product.RegularPrice
			if discounted {
				unitPrice = money(unitPrice * 0.7)
			}
			subtotal := money(unitPrice * float64(item.quantity))
			sale.Subtotal = money(sale.Subtotal + item.product.RegularPrice*float64(item.quantity))
			sale.TotalPaid = money(sale.TotalPaid + subtotal)
			sale.TotalDiscount = money(sale.TotalDiscount + item.product.RegularPrice*float64(item.quantity) - subtotal)
			item.lot.Available -= item.quantity
			detailID := data.NextIDs["detalle_venta"]
			if detailID < 1 {
				detailID = 1
			}
			data.NextIDs["detalle_venta"] = detailID + 1
			sale.Details = append(sale.Details, store.SaleDetail{
				ID: detailID, SaleID: saleID, LotID: item.lot.ID, Quantity: item.quantity,
				UnitPrice: unitPrice, Subtotal: subtotal, DiscountApplied: discounted,
			})
		}
		data.Sales = append(data.Sales, sale)
		return map[string]any{
			"mensaje": "Venta registrada correctamente", "venta_id": sale.ID,
			"total_pagado": sale.TotalPaid, "subtotal": sale.Subtotal, "descuento_total": sale.TotalDiscount,
		}, nil
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

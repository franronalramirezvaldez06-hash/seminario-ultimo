package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"minimarket/internal/store"
)

type Handler struct {
	repository *store.Repository
}

type apiError struct {
	status int
	detail string
}

func (e apiError) Error() string { return e.detail }

func New(repository *store.Repository) http.Handler {
	return &Handler{repository: repository}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.cors(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.cors(w, r)

	if r.URL.Path == "/" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]string{"app": "Minimarket API", "status": "online"})
		return
	}
	if r.URL.Path == "/api/health" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.URL.Path == "/api/productos" && r.Method == http.MethodGet:
		h.listProducts(w)
	case r.URL.Path == "/api/productos" && r.Method == http.MethodPost:
		h.createProduct(w, r)
	case r.URL.Path == "/api/inventario" && r.Method == http.MethodGet:
		h.listInventory(w)
	case r.URL.Path == "/api/lotes/ingresar" && r.Method == http.MethodPost:
		h.receiveLot(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/lotes/peps/") && r.Method == http.MethodGet:
		h.getFEFOLot(w, r)
	case r.URL.Path == "/api/lotes/proximos-vencer" && r.Method == http.MethodGet:
		h.listExpiringLots(w, r)
	case (r.URL.Path == "/api/ventas" || r.URL.Path == "/api/ventas/procesar") && r.Method == http.MethodPost:
		h.createSale(w, r)
	case r.URL.Path == "/api/mermas" && r.Method == http.MethodGet:
		h.listWaste(w)
	case r.URL.Path == "/api/mermas" && r.Method == http.MethodPost:
		h.createWaste(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) cors(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "http://localhost:3000" || origin == "http://127.0.0.1:3000" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func (h *Handler) listProducts(w http.ResponseWriter) {
	data, err := h.repository.Read()
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data.Products)
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string  `json:"nombre"`
		Category     string  `json:"categoria"`
		RegularPrice float64 `json:"precio_regular"`
	}
	if err := decodeBody(r, &input); err != nil {
		writeFailure(w, apiError{http.StatusBadRequest, "Solicitud JSON inválida."})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Category = strings.TrimSpace(input.Category)
	if len([]rune(input.Name)) < 2 || len([]rune(input.Category)) < 2 || !validPositivePrice(input.RegularPrice) {
		writeFailure(w, apiError{http.StatusBadRequest, "Nombre, categoría y precio válido son obligatorios."})
		return
	}

	result, err := h.repository.Update(func(data *store.Data) (any, error) {
		id := data.NextIDs["producto"]
		if id < 1 {
			id = 1
		}
		for findProductByBarcode(data, fmt.Sprintf("MINI-%08d", id)) != nil {
			id++
		}
		data.NextIDs["producto"] = id + 1
		now := time.Now().UTC()
		product := store.Product{
			ID: id, Barcode: fmt.Sprintf("MINI-%08d", id), Name: input.Name,
			Category: input.Category, RegularPrice: money(input.RegularPrice), CreatedAt: now.Format(time.RFC3339Nano),
		}
		data.Products = append(data.Products, product)
		return product, nil
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

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

func (h *Handler) listWaste(w http.ResponseWriter) {
	data, err := h.repository.Read()
	if err != nil {
		writeFailure(w, err)
		return
	}
	result := make([]map[string]any, 0, len(data.Waste))
	for i := len(data.Waste) - 1; i >= 0; i-- {
		entry := data.Waste[i]
		lot := findLotByID(&data, entry.LotID)
		name, lotNumber := "Producto eliminado", ""
		if lot != nil {
			lotNumber = lot.Number
			if product := findProductByID(&data, lot.ProductID); product != nil {
				name = product.Name
			}
		}
		result = append(result, map[string]any{
			"id": entry.ID, "lote_id": entry.LotID, "cantidad_retirada": entry.Quantity,
			"motivo": entry.Reason, "fecha_registro": entry.RecordedAt,
			"nombre_producto": name, "numero_lote": lotNumber,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) createWaste(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LotID    int    `json:"lote_id"`
		Quantity int    `json:"cantidad_retirada"`
		Reason   string `json:"motivo"`
	}
	if err := decodeBody(r, &input); err != nil {
		writeFailure(w, apiError{http.StatusBadRequest, "Solicitud JSON inválida."})
		return
	}
	input.Reason = strings.ToUpper(strings.TrimSpace(input.Reason))
	if input.Quantity < 1 || (input.Reason != "VENCIDO" && input.Reason != "DANADO" && input.Reason != "ROTO") {
		writeFailure(w, apiError{http.StatusBadRequest, "Cantidad inválida o motivo incorrecto. Usa VENCIDO, DANADO o ROTO."})
		return
	}

	result, err := h.repository.Update(func(data *store.Data) (any, error) {
		lot := findLotByID(data, input.LotID)
		if lot == nil {
			return nil, apiError{http.StatusNotFound, "Lote no encontrado."}
		}
		if input.Quantity > lot.Available {
			return nil, apiError{http.StatusBadRequest, "La cantidad retirada no puede exceder la cantidad disponible del lote."}
		}
		id := data.NextIDs["merma"]
		if id < 1 {
			id = 1
		}
		data.NextIDs["merma"] = id + 1
		lot.Available -= input.Quantity
		waste := store.Waste{
			ID: id, LotID: lot.ID, Quantity: input.Quantity, Reason: input.Reason,
			RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		data.Waste = append(data.Waste, waste)
		return map[string]any{
			"mensaje": "Merma registrada correctamente", "merma_id": waste.ID,
			"lote_id": waste.LotID, "cantidad_retirada": waste.Quantity,
			"motivo": waste.Reason, "fecha_registro": waste.RecordedAt,
		}, nil
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
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

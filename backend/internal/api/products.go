package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"minimarket/internal/store"
)

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

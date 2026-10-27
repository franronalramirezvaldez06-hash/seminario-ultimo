package api

import (
	"net/http"
	"strings"
	"time"

	"minimarket/internal/store"
)

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

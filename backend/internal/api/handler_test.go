package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"minimarket/internal/store"
)

func testServer(t *testing.T, data store.Data) (*store.Repository, http.Handler) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "store.json")
	content, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	repository := store.New(path)
	return repository, New(repository)
}

func testData() store.Data {
	today := time.Now().UTC()
	return store.Data{
		Products: []store.Product{
			{ID: 1, Barcode: "0123", Name: "Leche", Category: "Lácteos", RegularPrice: 1000},
		},
		Lots: []store.Lot{
			{ID: 1, ProductID: 1, Number: "LOT-LATE", Available: 5, ExpiresAt: today.AddDate(0, 0, 20).Format("2006-01-02") + "T00:00:00Z"},
			{ID: 2, ProductID: 1, Number: "LOT-FIRST", Available: 2, ExpiresAt: today.AddDate(0, 0, 3).Format("2006-01-02") + "T00:00:00Z"},
		},
		Sales: []store.Sale{},
		Waste: []store.Waste{},
		NextIDs: map[string]int{
			"producto": 2, "lote": 3, "venta": 1, "merma": 1, "detalle_venta": 1,
		},
	}
}

func TestFEFOLotAndDiscount(t *testing.T) {
	_, handler := testServer(t, testData())
	request := httptest.NewRequest(http.MethodGet, "/api/lotes/peps/0123", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	var result struct {
		LotID     int  `json:"id"`
		Discount  bool `json:"descuento_aplicado"`
		Available int  `json:"cantidad_disponible"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.LotID != 2 || !result.Discount || result.Available != 2 {
		t.Fatalf("unexpected FEFO lot: %+v", result)
	}
}

func TestSaleUsesFEFOAndRejectsInsufficientStockWithoutMutation(t *testing.T) {
	repository, handler := testServer(t, testData())
	request := httptest.NewRequest(http.MethodPost, "/api/ventas", bytes.NewBufferString(`[{"codigo_barras":"0123","cantidad":3}]`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body)
	}
	data, err := repository.Read()
	if err != nil {
		t.Fatal(err)
	}
	if data.Lots[0].Available != 5 || data.Lots[1].Available != 2 || len(data.Sales) != 0 {
		t.Fatalf("failed sale changed inventory: %+v", data)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/ventas", bytes.NewBufferString(`[{"codigo_barras":"0123","cantidad":1}]`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	data, err = repository.Read()
	if err != nil {
		t.Fatal(err)
	}
	if data.Lots[0].Available != 5 || data.Lots[1].Available != 1 || len(data.Sales) != 1 {
		t.Fatalf("successful sale did not use FEFO lot: %+v", data)
	}
	if data.Sales[0].TotalPaid != 700 || data.Sales[0].TotalDiscount != 300 {
		t.Fatalf("discount was not applied: %+v", data.Sales[0])
	}
}

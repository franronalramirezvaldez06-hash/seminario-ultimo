package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"minimarket/internal/store"
)

type testRepository struct {
	mu   sync.Mutex
	data store.Data
}

func testServer(t *testing.T, data store.Data) (*testRepository, http.Handler) {
	t.Helper()
	repository := &testRepository{data: cloneData(t, data)}
	return repository, New(repository)
}

func (r *testRepository) Read() (store.Data, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneData(nil, r.data), nil
}

func (r *testRepository) Update(change func(*store.Data) (any, error)) (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data := cloneData(nil, r.data)
	result, err := change(&data)
	if err != nil {
		return nil, err
	}
	r.data = data
	return result, nil
}

func cloneData(t *testing.T, data store.Data) store.Data {
	content, err := json.Marshal(data)
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		panic(err)
	}
	var result store.Data
	if err := json.Unmarshal(content, &result); err != nil {
		if t != nil {
			t.Fatal(err)
		}
		panic(err)
	}
	return result
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

func TestDashboardSummaryAggregatesExpiryAndDailySales(t *testing.T) {
	data := testData()
	today := utcToday()
	data.Products = append(data.Products,
		store.Product{ID: 2, Barcode: "0456", Name: "Yogur", Category: "Lácteos", RegularPrice: 500},
		store.Product{ID: 3, Barcode: "0789", Name: "Pan", Category: "Panadería", RegularPrice: 300},
	)
	data.Lots = append(data.Lots,
		store.Lot{ID: 3, ProductID: 1, Number: "LOT-EXPIRED-LECHE", Available: 2, ExpiresAt: today.AddDate(0, 0, -1).Format("2006-01-02")},
		store.Lot{ID: 4, ProductID: 2, Number: "LOT-EXPIRED-YOGUR", Available: 4, ExpiresAt: today.AddDate(0, 0, -1).Format("2006-01-02")},
		store.Lot{ID: 5, ProductID: 2, Number: "LOT-EXPIRED-YOGUR-2", Available: 1, ExpiresAt: today.AddDate(0, 0, -2).Format("2006-01-02")},
		store.Lot{ID: 6, ProductID: 1, Number: "LOT-EXPIRING-TODAY", Available: 1, ExpiresAt: today.Format("2006-01-02")},
		store.Lot{ID: 7, ProductID: 2, Number: "LOT-EXPIRING-LAST-DAY", Available: 2, ExpiresAt: today.AddDate(0, 0, 7).Format("2006-01-02")},
		store.Lot{ID: 8, ProductID: 2, Number: "LOT-EMPTY", Available: 0, ExpiresAt: today.AddDate(0, 0, -2).Format("2006-01-02")},
		store.Lot{ID: 9, ProductID: 3, Number: "LOT-OUTSIDE", Available: 2, ExpiresAt: today.AddDate(0, 0, 8).Format("2006-01-02")},
	)
	data.Sales = []store.Sale{
		{ID: 1, SoldAt: today.Format("2006-01-02") + "T12:00:00Z", TotalPaid: 1250},
		{ID: 2, SoldAt: today.AddDate(0, 0, -3).Format("2006-01-02") + "T10:00:00Z", TotalPaid: 500},
		{ID: 3, SoldAt: today.AddDate(0, 0, -8).Format("2006-01-02") + "T10:00:00Z", TotalPaid: 999},
	}
	_, handler := testServer(t, data)
	request := httptest.NewRequest(http.MethodGet, "/api/resumen", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	var result struct {
		TotalProducts    int     `json:"productos_total"`
		ExpiredProducts  int     `json:"productos_vencidos"`
		ExpiredUnits     int     `json:"unidades_vencidas"`
		ExpiringProducts int     `json:"productos_por_vencer"`
		ExpiringUnits    int     `json:"unidades_por_vencer"`
		TodaySaleCount   int     `json:"ventas_hoy"`
		TodaySalesTotal  float64 `json:"total_ventas_hoy"`
		SalesPerDay      []struct {
			Date      string  `json:"fecha"`
			SaleCount int     `json:"cantidad_ventas"`
			TotalPaid float64 `json:"total_ventas"`
		} `json:"ventas_por_dia"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TotalProducts != 3 || result.ExpiredProducts != 2 || result.ExpiredUnits != 7 {
		t.Fatalf("unexpected expired product summary: %+v", result)
	}
	if result.ExpiringProducts != 2 || result.ExpiringUnits != 5 {
		t.Fatalf("unexpected expiring product summary: %+v", result)
	}
	if result.TodaySaleCount != 1 || result.TodaySalesTotal != 1250 || len(result.SalesPerDay) != 7 {
		t.Fatalf("unexpected daily sale summary: %+v", result)
	}
	if first := result.SalesPerDay[0].Date; first != today.AddDate(0, 0, -6).Format("2006-01-02") {
		t.Fatalf("first chart date = %s, want start of seven-day period", first)
	}
	if last := result.SalesPerDay[6]; last.Date != today.Format("2006-01-02") || last.SaleCount != 1 || last.TotalPaid != 1250 {
		t.Fatalf("unexpected today's chart value: %+v", last)
	}
	if middle := result.SalesPerDay[3]; middle.SaleCount != 1 || middle.TotalPaid != 500 {
		t.Fatalf("unexpected historical chart value: %+v", middle)
	}
}

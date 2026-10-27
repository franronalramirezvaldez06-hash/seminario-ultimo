package api

import (
	"net/http"
	"strings"

	"minimarket/internal/store"
)

type Handler struct {
	repository dataRepository
}

type dataRepository interface {
	Read() (store.Data, error)
	Update(func(*store.Data) (any, error)) (any, error)
}

type apiError struct {
	status int
	detail string
}

func (e apiError) Error() string { return e.detail }

func New(repository dataRepository) http.Handler {
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
	case r.URL.Path == "/api/resumen" && r.Method == http.MethodGet:
		h.getDashboardSummary(w)
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

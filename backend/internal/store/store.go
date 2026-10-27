package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Product struct {
	ID           int     `json:"id"`
	Barcode      string  `json:"codigo_barras"`
	Name         string  `json:"nombre"`
	Category     string  `json:"categoria"`
	RegularPrice float64 `json:"precio_regular"`
	CreatedAt    string  `json:"creado_en,omitempty"`
}

type Lot struct {
	ID         int    `json:"id"`
	ProductID  int    `json:"producto_id"`
	Number     string `json:"numero_lote"`
	Available  int    `json:"cantidad_disponible"`
	ExpiresAt  string `json:"fecha_vencimiento"`
	ReceivedAt string `json:"fecha_ingreso"`
}

type SaleDetail struct {
	ID              int     `json:"id"`
	SaleID          int     `json:"venta_id"`
	LotID           int     `json:"lote_id"`
	Quantity        int     `json:"cantidad"`
	UnitPrice       float64 `json:"precio_unitario"`
	Subtotal        float64 `json:"subtotal"`
	DiscountApplied bool    `json:"descuento_aplicado"`
}

type Sale struct {
	ID            int          `json:"id"`
	SoldAt        string       `json:"fecha_venta"`
	TotalPaid     float64      `json:"total_pagado"`
	Subtotal      float64      `json:"subtotal"`
	TotalDiscount float64      `json:"descuento_total"`
	Details       []SaleDetail `json:"detalles"`
}

type Waste struct {
	ID         int    `json:"id"`
	LotID      int    `json:"lote_id"`
	Quantity   int    `json:"cantidad_retirada"`
	Reason     string `json:"motivo"`
	RecordedAt string `json:"fecha_registro"`
}

type Data struct {
	Products []Product      `json:"productos"`
	Lots     []Lot          `json:"lotes"`
	Sales    []Sale         `json:"ventas"`
	Waste    []Waste        `json:"mermas"`
	NextIDs  map[string]int `json:"next_ids"`
}

type Repository struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Repository {
	return &Repository{path: path}
}

func (r *Repository) Check() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.read()
	return err
}

func (r *Repository) Read() (Data, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.read()
}

func (r *Repository) Update(change func(*Data) (any, error)) (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.read()
	if err != nil {
		return nil, err
	}
	result, err := change(&data)
	if err != nil {
		return nil, err
	}
	if err := r.write(data); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) read() (Data, error) {
	file, err := os.Open(r.path)
	if err != nil {
		return Data{}, fmt.Errorf("open %s: %w", r.path, err)
	}
	defer file.Close()

	var data Data
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return Data{}, fmt.Errorf("decode %s: %w", r.path, err)
	}
	if data.Products == nil || data.Lots == nil || data.Sales == nil || data.Waste == nil || data.NextIDs == nil {
		return Data{}, errors.New("inventory data is missing required collections")
	}
	for i := range data.Products {
		if data.Products[i].Category == "" {
			data.Products[i].Category = "General"
		}
	}
	return data, nil
}

func (r *Repository) write(data Data) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(r.path), "store-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary data file: %w", err)
	}
	defer os.Remove(file.Name())

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		file.Close()
		return fmt.Errorf("encode inventory data: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("flush inventory data: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close inventory data: %w", err)
	}
	if err := os.Rename(file.Name(), r.path); err != nil {
		return fmt.Errorf("replace inventory data: %w", err)
	}
	return nil
}

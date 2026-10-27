package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const advisoryLockID int64 = 748193625014

var sequenceNames = []string{"producto", "lote", "venta", "merma", "detalle_venta"}

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
	db *sql.DB
	mu sync.Mutex
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin database schema migration: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE SCHEMA IF NOT EXISTS minimarket`,
		`CREATE TABLE IF NOT EXISTS minimarket.schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS minimarket.products (
			id BIGINT PRIMARY KEY,
			barcode TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			regular_price DOUBLE PRECISION NOT NULL,
			created_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS minimarket.lots (
			id BIGINT PRIMARY KEY,
			product_id BIGINT NOT NULL REFERENCES minimarket.products(id),
			number TEXT NOT NULL UNIQUE,
			available INTEGER NOT NULL CHECK (available >= 0),
			expires_at DATE NOT NULL,
			received_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS minimarket.sales (
			id BIGINT PRIMARY KEY,
			sold_at TIMESTAMPTZ NOT NULL,
			total_paid DOUBLE PRECISION NOT NULL,
			subtotal DOUBLE PRECISION NOT NULL,
			total_discount DOUBLE PRECISION NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS minimarket.sale_details (
			id BIGINT PRIMARY KEY,
			sale_id BIGINT NOT NULL REFERENCES minimarket.sales(id),
			lot_id BIGINT NOT NULL REFERENCES minimarket.lots(id),
			quantity INTEGER NOT NULL CHECK (quantity > 0),
			unit_price DOUBLE PRECISION NOT NULL,
			subtotal DOUBLE PRECISION NOT NULL,
			discount_applied BOOLEAN NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS minimarket.waste_entries (
			id BIGINT PRIMARY KEY,
			lot_id BIGINT NOT NULL REFERENCES minimarket.lots(id),
			quantity INTEGER NOT NULL CHECK (quantity > 0),
			reason TEXT NOT NULL,
			recorded_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS minimarket.app_sequences (
			name TEXT PRIMARY KEY,
			next_id BIGINT NOT NULL CHECK (next_id > 0)
		)`,
		`INSERT INTO minimarket.schema_migrations (version) VALUES (1) ON CONFLICT (version) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply database schema migration: %w", err)
		}
	}
	for _, name := range sequenceNames {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO minimarket.app_sequences (name, next_id) VALUES ($1, 1) ON CONFLICT (name) DO NOTHING`, name,
		); err != nil {
			return fmt.Errorf("initialize %s ID sequence: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit database schema migration: %w", err)
	}
	return nil
}

func (r *Repository) ImportJSONIfEmpty(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open legacy data file %s: %w", path, err)
	}
	defer file.Close()

	var data Data
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return false, fmt.Errorf("decode legacy data file %s: %w", path, err)
	}
	if data.Products == nil || data.Lots == nil || data.Sales == nil || data.Waste == nil || data.NextIDs == nil {
		return false, errors.New("legacy data is missing required collections")
	}
	for i := range data.Products {
		if data.Products[i].Category == "" {
			data.Products[i].Category = "General"
		}
	}
	return r.ImportIfEmpty(data)
}

func (r *Repository) ImportIfEmpty(data Data) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin legacy data import: %w", err)
	}
	defer tx.Rollback()
	if err := lockRepository(ctx, tx); err != nil {
		return false, err
	}

	var records int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM minimarket.products) +
			(SELECT COUNT(*) FROM minimarket.lots) +
			(SELECT COUNT(*) FROM minimarket.sales) +
			(SELECT COUNT(*) FROM minimarket.sale_details) +
			(SELECT COUNT(*) FROM minimarket.waste_entries)
	`).Scan(&records); err != nil {
		return false, fmt.Errorf("check database before legacy data import: %w", err)
	}
	if records != 0 {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("finish legacy data import check: %w", err)
		}
		return false, nil
	}
	if err := writeData(ctx, tx, data); err != nil {
		return false, fmt.Errorf("import legacy data: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit legacy data import: %w", err)
	}
	return true, nil
}

func (r *Repository) Check() error {
	_, err := r.Read()
	return err
}

func (r *Repository) Read() (Data, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Data{}, fmt.Errorf("begin database read: %w", err)
	}
	defer tx.Rollback()
	data, err := readData(ctx, tx)
	if err != nil {
		return Data{}, err
	}
	if err := tx.Commit(); err != nil {
		return Data{}, fmt.Errorf("finish database read: %w", err)
	}
	return data, nil
}

func (r *Repository) Update(change func(*Data) (any, error)) (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin database update: %w", err)
	}
	defer tx.Rollback()
	if err := lockRepository(ctx, tx); err != nil {
		return nil, err
	}
	data, err := readData(ctx, tx)
	if err != nil {
		return nil, err
	}
	result, err := change(&data)
	if err != nil {
		return nil, err
	}
	if err := writeData(ctx, tx, data); err != nil {
		return nil, fmt.Errorf("save database update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit database update: %w", err)
	}
	return result, nil
}

type databaseReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readData(ctx context.Context, query databaseReader) (Data, error) {
	data := Data{
		Products: []Product{},
		Lots:     []Lot{},
		Sales:    []Sale{},
		Waste:    []Waste{},
		NextIDs:  make(map[string]int, len(sequenceNames)),
	}

	rows, err := query.QueryContext(ctx, `
		SELECT id, barcode, name, category, regular_price, created_at
		FROM minimarket.products ORDER BY id
	`)
	if err != nil {
		return Data{}, fmt.Errorf("read products: %w", err)
	}
	for rows.Next() {
		var product Product
		var createdAt sql.NullTime
		if err := rows.Scan(&product.ID, &product.Barcode, &product.Name, &product.Category, &product.RegularPrice, &createdAt); err != nil {
			rows.Close()
			return Data{}, fmt.Errorf("scan product: %w", err)
		}
		if createdAt.Valid {
			product.CreatedAt = createdAt.Time.UTC().Format(time.RFC3339Nano)
		}
		data.Products = append(data.Products, product)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Data{}, fmt.Errorf("iterate products: %w", err)
	}
	rows.Close()

	rows, err = query.QueryContext(ctx, `
		SELECT id, product_id, number, available, expires_at, received_at
		FROM minimarket.lots ORDER BY id
	`)
	if err != nil {
		return Data{}, fmt.Errorf("read lots: %w", err)
	}
	for rows.Next() {
		var lot Lot
		var expiresAt time.Time
		var receivedAt sql.NullTime
		if err := rows.Scan(&lot.ID, &lot.ProductID, &lot.Number, &lot.Available, &expiresAt, &receivedAt); err != nil {
			rows.Close()
			return Data{}, fmt.Errorf("scan lot: %w", err)
		}
		lot.ExpiresAt = expiresAt.Format("2006-01-02") + "T00:00:00Z"
		if receivedAt.Valid {
			lot.ReceivedAt = receivedAt.Time.UTC().Format(time.RFC3339Nano)
		}
		data.Lots = append(data.Lots, lot)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Data{}, fmt.Errorf("iterate lots: %w", err)
	}
	rows.Close()

	rows, err = query.QueryContext(ctx, `
		SELECT id, sold_at, total_paid, subtotal, total_discount
		FROM minimarket.sales ORDER BY id
	`)
	if err != nil {
		return Data{}, fmt.Errorf("read sales: %w", err)
	}
	salesByID := make(map[int]int)
	for rows.Next() {
		var sale Sale
		var soldAt time.Time
		if err := rows.Scan(&sale.ID, &soldAt, &sale.TotalPaid, &sale.Subtotal, &sale.TotalDiscount); err != nil {
			rows.Close()
			return Data{}, fmt.Errorf("scan sale: %w", err)
		}
		sale.SoldAt = soldAt.UTC().Format(time.RFC3339Nano)
		sale.Details = []SaleDetail{}
		salesByID[sale.ID] = len(data.Sales)
		data.Sales = append(data.Sales, sale)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Data{}, fmt.Errorf("iterate sales: %w", err)
	}
	rows.Close()

	rows, err = query.QueryContext(ctx, `
		SELECT id, sale_id, lot_id, quantity, unit_price, subtotal, discount_applied
		FROM minimarket.sale_details ORDER BY id
	`)
	if err != nil {
		return Data{}, fmt.Errorf("read sale details: %w", err)
	}
	for rows.Next() {
		var detail SaleDetail
		if err := rows.Scan(&detail.ID, &detail.SaleID, &detail.LotID, &detail.Quantity,
			&detail.UnitPrice, &detail.Subtotal, &detail.DiscountApplied); err != nil {
			rows.Close()
			return Data{}, fmt.Errorf("scan sale detail: %w", err)
		}
		index, exists := salesByID[detail.SaleID]
		if !exists {
			rows.Close()
			return Data{}, fmt.Errorf("sale detail %d references missing sale %d", detail.ID, detail.SaleID)
		}
		data.Sales[index].Details = append(data.Sales[index].Details, detail)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Data{}, fmt.Errorf("iterate sale details: %w", err)
	}
	rows.Close()

	rows, err = query.QueryContext(ctx, `
		SELECT id, lot_id, quantity, reason, recorded_at
		FROM minimarket.waste_entries ORDER BY id
	`)
	if err != nil {
		return Data{}, fmt.Errorf("read waste entries: %w", err)
	}
	for rows.Next() {
		var waste Waste
		var recordedAt time.Time
		if err := rows.Scan(&waste.ID, &waste.LotID, &waste.Quantity, &waste.Reason, &recordedAt); err != nil {
			rows.Close()
			return Data{}, fmt.Errorf("scan waste entry: %w", err)
		}
		waste.RecordedAt = recordedAt.UTC().Format(time.RFC3339Nano)
		data.Waste = append(data.Waste, waste)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Data{}, fmt.Errorf("iterate waste entries: %w", err)
	}
	rows.Close()

	rows, err = query.QueryContext(ctx, `SELECT name, next_id FROM minimarket.app_sequences`)
	if err != nil {
		return Data{}, fmt.Errorf("read ID sequences: %w", err)
	}
	for rows.Next() {
		var name string
		var nextID int
		if err := rows.Scan(&name, &nextID); err != nil {
			rows.Close()
			return Data{}, fmt.Errorf("scan ID sequence: %w", err)
		}
		data.NextIDs[name] = nextID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Data{}, fmt.Errorf("iterate ID sequences: %w", err)
	}
	rows.Close()

	return data, nil
}

func writeData(ctx context.Context, tx *sql.Tx, data Data) error {
	for _, table := range []string{
		"minimarket.waste_entries", "minimarket.sale_details", "minimarket.sales",
		"minimarket.lots", "minimarket.products",
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	for _, product := range data.Products {
		category := product.Category
		if strings.TrimSpace(category) == "" {
			category = "General"
		}
		createdAt, err := nullableTimestamp(product.CreatedAt)
		if err != nil {
			return fmt.Errorf("parse creation date for product %d: %w", product.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO minimarket.products (id, barcode, name, category, regular_price, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, product.ID, product.Barcode, product.Name, category, product.RegularPrice, createdAt); err != nil {
			return fmt.Errorf("write product %d: %w", product.ID, err)
		}
	}
	for _, lot := range data.Lots {
		expiresAt, err := parseDate(lot.ExpiresAt)
		if err != nil {
			return fmt.Errorf("parse expiry for lot %d: %w", lot.ID, err)
		}
		receivedAt, err := nullableTimestamp(lot.ReceivedAt)
		if err != nil {
			return fmt.Errorf("parse receipt date for lot %d: %w", lot.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO minimarket.lots (id, product_id, number, available, expires_at, received_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, lot.ID, lot.ProductID, lot.Number, lot.Available, expiresAt, receivedAt); err != nil {
			return fmt.Errorf("write lot %d: %w", lot.ID, err)
		}
	}
	for _, sale := range data.Sales {
		soldAt, err := parseTimestamp(sale.SoldAt)
		if err != nil {
			return fmt.Errorf("parse date for sale %d: %w", sale.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO minimarket.sales (id, sold_at, total_paid, subtotal, total_discount)
			VALUES ($1, $2, $3, $4, $5)
		`, sale.ID, soldAt, sale.TotalPaid, sale.Subtotal, sale.TotalDiscount); err != nil {
			return fmt.Errorf("write sale %d: %w", sale.ID, err)
		}
		for _, detail := range sale.Details {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO minimarket.sale_details
					(id, sale_id, lot_id, quantity, unit_price, subtotal, discount_applied)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, detail.ID, detail.SaleID, detail.LotID, detail.Quantity, detail.UnitPrice, detail.Subtotal, detail.DiscountApplied); err != nil {
				return fmt.Errorf("write sale detail %d: %w", detail.ID, err)
			}
		}
	}
	for _, waste := range data.Waste {
		recordedAt, err := parseTimestamp(waste.RecordedAt)
		if err != nil {
			return fmt.Errorf("parse date for waste entry %d: %w", waste.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO minimarket.waste_entries (id, lot_id, quantity, reason, recorded_at)
			VALUES ($1, $2, $3, $4, $5)
		`, waste.ID, waste.LotID, waste.Quantity, waste.Reason, recordedAt); err != nil {
			return fmt.Errorf("write waste entry %d: %w", waste.ID, err)
		}
	}

	for _, name := range sequenceNames {
		nextID := data.NextIDs[name]
		if nextID < 1 {
			nextID = 1
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO minimarket.app_sequences (name, next_id) VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET next_id = EXCLUDED.next_id
		`, name, nextID); err != nil {
			return fmt.Errorf("write %s ID sequence: %w", name, err)
		}
	}
	return nil
}

func lockRepository(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("lock database update: %w", err)
	}
	return nil
}

func nullableTimestamp(value string) (any, error) {
	if value == "" {
		return nil, nil
	}
	return parseTimestamp(value)
}

func parseTimestamp(value string) (time.Time, error) {
	if timestamp, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return timestamp, nil
	}
	timestamp, err := time.ParseInLocation("2006-01-02T15:04:05.999999999", value, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp %q", value)
	}
	return timestamp, nil
}

func parseDate(value string) (time.Time, error) {
	if len(value) < len("2006-01-02") {
		return time.Time{}, fmt.Errorf("invalid date %q", value)
	}
	date, err := time.Parse("2006-01-02", value[:10])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q", value)
	}
	return date, nil
}

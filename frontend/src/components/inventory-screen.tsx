"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { getInventory, getProducts, receiveLot } from "@/lib/api";
import { formatCurrency, formatDate } from "@/lib/format";
import type { InventoryProduct, Product } from "@/lib/types";

export function InventoryScreen() {
  const [products, setProducts] = useState<Product[]>([]);
  const [inventory, setInventory] = useState<InventoryProduct[]>([]);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState(false);
  const [expiry, setExpiry] = useState("");

  const loadInventory = useCallback(() => Promise.all([getProducts(), getInventory()]), []);
  const applyInventory = useCallback(([catalog, stock]: [Product[], InventoryProduct[]]) => {
    setProducts(catalog);
    setInventory(stock);
    setError(false);
  }, []);
  const showLoadError = useCallback((reason: unknown) => {
    setMessage(reason instanceof Error ? reason.message : "No se pudo cargar el inventario.");
    setError(true);
  }, []);

  useEffect(() => {
    let active = true;
    const load = () => {
      void loadInventory()
        .then((data) => { if (active) applyInventory(data); })
        .catch((reason: unknown) => { if (active) showLoadError(reason); })
        .finally(() => { if (active) setLoading(false); });
    };
    load();
    const onInventoryUpdate = () => { setLoading(true); load(); };
    window.addEventListener("inventory-updated", onInventoryUpdate);
    return () => {
      active = false;
      window.removeEventListener("inventory-updated", onInventoryUpdate);
    };
  }, [applyInventory, loadInventory, showLoadError]);

  function refresh() {
    setLoading(true);
    void loadInventory()
      .then(applyInventory)
      .catch(showLoadError)
      .finally(() => setLoading(false));
  }

  const filtered = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();
    if (!query) return inventory;
    return inventory.filter((product) =>
      `${product.name} ${product.category} ${product.barcode}`.toLocaleLowerCase().includes(query),
    );
  }, [inventory, search]);

  async function submitLot(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    setSaving(true);
    setMessage("");
    try {
      await receiveLot({
        productId: Number(values.get("productId")),
        quantity: Number(values.get("quantity")),
        expiresAt: String(values.get("expiry")),
      });
      setMessage("Lote registrado correctamente.");
      setError(false);
      form.reset();
      setExpiry("");
      await refresh();
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "No se pudo registrar el lote.");
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="page">
      <header className="page-heading">
        <div>
          <p className="eyebrow">Panel de control</p>
          <h1>Inventario</h1>
          <p>Consulta tus productos, existencias y próximos vencimientos.</p>
        </div>
        <div className="inventory-summary"><span className="summary-icon">▦</span><div><strong>{inventory.length}</strong><span>productos registrados</span></div></div>
      </header>

      <section className="card">
        <header className="card-heading">
          <div className="heading-mark">+</div>
          <div><p className="eyebrow">Control de inventario</p><h2>Ingreso de lote</h2><p>Registra existencias y controla su fecha de vencimiento.</p></div>
        </header>
        <form className="lot-form" onSubmit={submitLot}>
          <div className="fields-grid">
            <div className="field">
              <label htmlFor="lot-product">Producto <span className="required-mark">*</span></label>
              <select id="lot-product" name="productId" defaultValue="" required disabled={!products.length}>
                <option value="" disabled>{products.length ? "Selecciona un producto" : "Crea un producto para comenzar"}</option>
                {products.map((product) => <option key={product.id} value={product.id}>{product.name} · #{product.id}</option>)}
              </select>
              <span className="field-hint">Selecciona el producto que recibiste.</span>
            </div>
            <div className="field">
              <label htmlFor="lot-quantity">Cantidad <span className="required-mark">*</span></label>
              <input id="lot-quantity" name="quantity" type="number" min="1" step="1" defaultValue="1" required />
              <span className="field-hint">Unidades recibidas.</span>
            </div>
            <div className="field">
              <label htmlFor="lot-expiry">Fecha de vencimiento <span className="required-mark">*</span></label>
              <input id="lot-expiry" name="expiry" type="date" value={expiry} onChange={(event) => setExpiry(event.target.value)} required />
            </div>
          </div>
          <div className="form-actions"><span className="required-note"><span className="required-mark">*</span> Campos obligatorios</span><button className="primary-button save-button" type="submit" disabled={saving || !products.length}>{saving ? "Guardando…" : "Guardar lote"} <span>→</span></button></div>
        </form>
        {message && <p className={`message${error ? " message-error" : ""}`} role={error ? "alert" : "status"}>{message}</p>}
      </section>

      <section className="card inventory-card">
        <header className="inventory-heading">
          <div><p className="eyebrow">Existencias</p><h2>Productos en inventario</h2><p>Unidades disponibles sumadas en los lotes activos.</p></div>
          <div className="inventory-tools">
            <label className="search-box"><span aria-hidden="true">⌕</span><input type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Buscar producto…" /></label>
            <button type="button" className="secondary-button" disabled={loading} onClick={refresh}>↻ Actualizar</button>
          </div>
        </header>
        <div className="table-wrap">
          <table>
            <thead><tr><th>Producto</th><th>Código</th><th>Categoría</th><th>Precio</th><th>Stock</th><th>Próximo vencimiento</th></tr></thead>
            <tbody>
              {filtered.map((product) => (
                <tr key={product.id}>
                  <td><span className="product-avatar">{product.name.slice(0, 1).toUpperCase()}</span><span className="product-name"><strong>{product.name}</strong><small>ID #{product.id}</small></span></td>
                  <td className="barcode-cell">{product.barcode}</td>
                  <td><span className="category-badge">{product.category}</span></td>
                  <td>{formatCurrency(product.regularPrice)}</td>
                  <td><span className={`stock-badge${product.available === 0 ? " stock-empty" : product.available < 6 ? " stock-low" : ""}`}>{product.available} <small>unid.</small></span></td>
                  <td>{product.nextExpiry ? formatDate(product.nextExpiry) : <span className="muted">Sin existencias</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {!loading && filtered.length === 0 && <div className="empty-state"><span className="empty-mark">⌕</span><strong>{search ? "No hay coincidencias" : "No hay productos para mostrar"}</strong><span>{search ? "Prueba con otro nombre, categoría o código." : "Crea un producto y registra un lote para iniciar."}</span></div>}
          {loading && <div className="empty-state"><span className="empty-mark">↻</span><strong>Cargando inventario…</strong></div>}
        </div>
        {!loading && inventory.length > 0 && <footer className="inventory-footer">Mostrando {filtered.length} de {inventory.length} productos</footer>}
      </section>
    </section>
  );
}

"use client";

import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { createWaste, getExpiringLots, getFEFOLot, getProducts, getWaste, notifyInventoryChanged } from "@/lib/api";
import { daysLabel, formatDate } from "@/lib/format";
import type { ExpiringLot, Lot, Product, WasteEntry, WasteReason } from "@/lib/types";

export function WasteScreen() {
  const [products, setProducts] = useState<Product[]>([]);
  const [entries, setEntries] = useState<WasteEntry[]>([]);
  const [expiring, setExpiring] = useState<ExpiringLot[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [productId, setProductId] = useState("");
  const [lot, setLot] = useState<Lot | null>(null);
  const [loadingLot, setLoadingLot] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState(false);
  const lotRequest = useRef(0);

  const loadWasteData = useCallback(() => Promise.all([getProducts(), getWaste(), getExpiringLots()]), []);
  const applyWasteData = useCallback(([catalog, history, alerts]: [Product[], WasteEntry[], ExpiringLot[]]) => {
    setProducts(catalog);
    setEntries(history);
    setExpiring(alerts);
  }, []);
  const showLoadError = useCallback((reason: unknown) => {
    setMessage(reason instanceof Error ? reason.message : "No se pudo cargar el registro de mermas.");
    setError(true);
  }, []);

  useEffect(() => {
    let active = true;
    const load = () => {
      void loadWasteData()
        .then((data) => { if (active) applyWasteData(data); })
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
  }, [applyWasteData, loadWasteData, showLoadError]);

  async function selectProduct(id: string) {
    const requestId = ++lotRequest.current;
    setProductId(id);
    setLot(null);
    setLoadingLot(Boolean(id));
    if (!id) return;
    const product = products.find((item) => item.id === Number(id));
    if (!product) {
      setLoadingLot(false);
      return;
    }
    try {
      const selectedLot = await getFEFOLot(product.barcode);
      if (requestId === lotRequest.current) setLot(selectedLot);
    } catch (reason) {
      if (requestId === lotRequest.current) {
        setMessage(reason instanceof Error ? reason.message : "No hay existencias disponibles para este producto.");
        setError(true);
      }
    } finally {
      if (requestId === lotRequest.current) setLoadingLot(false);
    }
  }

  function openForm(forProduct?: number) {
    setMessage("");
    setDialogOpen(true);
    void selectProduct(forProduct ? String(forProduct) : "");
  }

  async function submitWaste(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!lot) {
      setMessage("Selecciona un producto con existencias disponibles.");
      setError(true);
      return;
    }


    const values = new FormData(event.currentTarget);
    const quantity = Number(values.get("quantity"));
    if (!Number.isInteger(quantity) || quantity < 1 || quantity > lot.available) {
      setMessage(`La cantidad debe estar entre 1 y ${lot.available}.`);
      setError(true);
      return;
    }
    setSaving(true);
    try {
      await createWaste({ lotId: lot.id, quantity, reason: String(values.get("reason")) as WasteReason });
      setMessage(`Merma registrada: ${quantity} unidad(es) de ${lot.name}.`);
      setError(false);
      setDialogOpen(false);
      lotRequest.current++;
      setProductId("");
      setLot(null);
      setLoadingLot(false);
      notifyInventoryChanged();
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "No se pudo registrar la merma.");
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="page">
      <header className="page-heading">
        <div className="page-heading-mark waste-mark">!</div>
        <div><p className="eyebrow waste-eyebrow">Control de inventario</p><h1>Registro de mermas</h1><p>Controla bajas y consulta el historial de unidades retiradas.</p></div>
        <button className="primary-button open-waste-form" type="button" onClick={() => openForm()}>Registrar nueva merma</button>
      </header>
      {message && <p className={`message${error ? " message-error" : ""}`} role={error ? "alert" : "status"}>{message}</p>}

      <section className="card expiry-card">
        <header className="expiry-heading">
          <div className="section-title"><span className="expiry-icon">!</span><div><p className="eyebrow expiry-eyebrow">Atención de inventario</p><h2>Productos vencidos o próximos a vencer</h2><p>Productos con existencias y fecha de vencimiento dentro de 7 días.</p></div></div>
          <span className="expiry-count">{expiring.length} lotes</span>
        </header>
        {loading ? <div className="empty-state">Consultando fechas de vencimiento…</div> : expiring.length ? (
          <div className="table-wrap"><table>
            <thead><tr><th>Producto</th><th>Lote</th><th>Stock</th><th>Vencimiento</th><th>Estado</th><th><span className="visually-hidden">Acción</span></th></tr></thead>
            <tbody>{expiring.map((item) => (
              <tr key={item.lotId}>
                <td><strong>{item.productName}</strong><small className="table-subtext">{item.category}</small></td>
                <td>{item.lotNumber}</td><td>{item.available} unid.</td><td>{formatDate(item.expiresAt)}</td>
                <td><span className={`expiry-badge${item.daysToExpiry < 0 ? " expired" : ""}`}>{daysLabel(item.daysToExpiry)}</span></td>
                <td><button className="text-button" type="button" onClick={() => openForm(item.productId)}>Revisar merma</button></td>
              </tr>
            ))}</tbody>
          </table></div>
        ) : <div className="empty-state"><span className="empty-mark">✓</span><strong>No hay productos próximos a vencer</strong><span>Los lotes con vencimiento dentro de 7 días aparecerán aquí.</span></div>}
      </section>

      {dialogOpen && (
        <div className="modal-backdrop" onMouseDown={(event) => {
          if (event.target === event.currentTarget && !saving) setDialogOpen(false);
        }}>
          <section className="card waste-modal" role="dialog" aria-modal="true" aria-labelledby="waste-form-title">
            <header className="modal-heading">
              <div><p className="eyebrow waste-eyebrow">Salida de inventario</p><h2 id="waste-form-title">Registrar nueva merma</h2><p>Se descontará del lote que vence primero.</p></div>
              <button className="modal-close" type="button" aria-label="Cerrar formulario de merma" disabled={saving} onClick={() => setDialogOpen(false)}>×</button>
            </header>
            <form className="product-form" onSubmit={submitWaste}>
              <div className="field">
                <label htmlFor="waste-product">Producto <span className="required-mark">*</span></label>
                <select id="waste-product" value={productId} onChange={(event) => void selectProduct(event.target.value)} required>
                  <option value="">Selecciona un producto</option>
                  {products.map((product) => <option key={product.id} value={product.id}>{product.name} · {product.category}</option>)}
                </select>
                <span className="field-hint">Se retirará del lote disponible que vence primero.</span>
              </div>
              {loadingLot && <div className="lot-info">Consultando lote disponible…</div>}
              {lot && <div className="lot-info"><span className="lot-info-icon">✓</span><p><strong>{lot.name}</strong><span>{lot.number} · {lot.available} unidades disponibles · vence {formatDate(lot.expiresAt)}</span></p></div>}
              <div className="fields-grid two-columns">
                <div className="field"><label htmlFor="waste-quantity">Cantidad a retirar <span className="required-mark">*</span></label><input id="waste-quantity" name="quantity" type="number" min="1" max={lot?.available} step="1" defaultValue="1" required /><span className="field-hint">{lot ? `Máximo: ${lot.available} unidades.` : "Unidades que se darán de baja."}</span></div>
                <div className="field"><label htmlFor="waste-reason">Motivo <span className="required-mark">*</span></label><select id="waste-reason" name="reason" defaultValue="VENCIDO" required><option value="VENCIDO">Vencido</option><option value="DANADO">Dañado</option><option value="ROTO">Roto</option></select></div>
              </div>
              {message && error && <p className="message message-error" role="alert">{message}</p>}
              <footer className="modal-actions"><span className="required-note"><span className="required-mark">*</span> Campos obligatorios</span><button className="primary-button" type="submit" disabled={saving || loadingLot || !lot}>{saving ? "Registrando…" : "Registrar merma"} →</button></footer>
            </form>
          </section>
        </div>
      )}

      <section className="card history-card">
        <header className="inventory-heading"><div><p className="eyebrow waste-eyebrow">Auditoría</p><h2>Historial de mermas</h2></div><span className="record-count">{entries.length} registros</span></header>
        {loading ? <div className="empty-state">Cargando historial…</div> : entries.length ? (
          <div className="table-wrap"><table>
            <thead><tr><th>Fecha</th><th>Producto / lote</th><th>Cantidad</th><th>Motivo</th></tr></thead>
            <tbody>{entries.map((entry) => (
              <tr key={entry.id}><td>{formatDate(entry.recordedAt)}</td><td><strong>{entry.productName}</strong><small className="table-subtext">{entry.lotNumber || `Lote #${entry.lotId}`}</small></td><td>{entry.quantity} unid.</td><td><span className="category-badge">{entry.reason}</span></td></tr>
            ))}</tbody>
          </table></div>
        ) : <div className="empty-state"><span className="empty-mark">▤</span><strong>Aún no hay registros de mermas</strong><span>Las bajas de inventario aparecerán aquí.</span></div>}
      </section>
    </section>
  );
}

"use client";

import { useEffect, useMemo, useState, type FormEvent } from "react";
import { createSale, getFEFOLot, getInventory, getProducts, notifyInventoryChanged } from "@/lib/api";
import { formatCurrency, formatDate } from "@/lib/format";
import type { Lot, Product } from "@/lib/types";

interface CartItem {
  barcode: string;
  name: string;
  quantity: number;
  stock: number;
  regularPrice: number;
  price: number;
  discount: boolean;
}

export function POSScreen() {
  const [products, setProducts] = useState<Product[]>([]);
  const [productId, setProductId] = useState("");
  const [quantity, setQuantity] = useState(1);
  const [cart, setCart] = useState<CartItem[]>([]);
  const [lastLot, setLastLot] = useState<Lot | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState(false);

  useEffect(() => {
    const loadProducts = async () => {
      try {
        const [catalog, inventory] = await Promise.all([getProducts(), getInventory()]);
        const availableProducts = catalog.filter((product) => {
          const stock = inventory.find((item) => item.id === product.id)?.available ?? 0;
          return stock > 0;
        });
        setProducts(availableProducts);
        if (productId && !availableProducts.some((product) => String(product.id) === productId)) {
          setProductId("");
          setLastLot(null);
        }
      } catch (reason) {
        setMessage(reason instanceof Error ? reason.message : "No se pudo cargar el catálogo.");
        setError(true);
      } finally {
        setLoading(false);
      }
    };

    void loadProducts();
    const onInventoryUpdate = () => {
      setLoading(true);
      void loadProducts();
    };
    window.addEventListener("inventory-updated", onInventoryUpdate);
    return () => window.removeEventListener("inventory-updated", onInventoryUpdate);
  }, [productId]);

  const selectedProduct = useMemo(
    () => products.find((item) => item.id === Number(productId)) ?? null,
    [productId, products],
  );

  const inCartForSelection = useMemo(() => {
    if (!lastLot) return 0;
    return cart.filter((item) => item.barcode === lastLot.barcode).reduce((sum, item) => sum + item.quantity, 0);
  }, [cart, lastLot]);

  const availableForSelection = useMemo(() => {
    if (!lastLot) return 0;
    return Math.max(0, lastLot.available - inCartForSelection);
  }, [inCartForSelection, lastLot]);

  const canAddSelection = Boolean(selectedProduct) && availableForSelection > 0;
  const quantityLimit = availableForSelection > 0 ? availableForSelection : 1;

  const subtotal = useMemo(() => cart.reduce((sum, item) => sum + item.regularPrice * item.quantity, 0), [cart]);
  const discount = useMemo(() => cart.reduce((sum, item) => sum + (item.regularPrice - item.price) * item.quantity, 0), [cart]);
  const total = useMemo(() => cart.reduce((sum, item) => sum + item.price * item.quantity, 0), [cart]);

  useEffect(() => {
    if (!selectedProduct) {
      setLastLot(null);
      setQuantity(1);
      return;
    }

    let cancelled = false;
    const refreshSelectedLot = async () => {
      try {
        const lot = await getFEFOLot(selectedProduct.barcode);
        if (!cancelled) {
          setLastLot(lot);
          if (lot.available <= 0) {
            setQuantity(1);
          }
        }
      } catch (reason) {
        if (!cancelled) {
          setLastLot(null);
          if (reason instanceof Error) {
            setMessage(reason.message);
          } else {
            setMessage("No hay un lote disponible.");
          }
          setError(true);
        }
      }
    };

    void refreshSelectedLot();
    return () => {
      cancelled = true;
    };
  }, [selectedProduct]);

  async function findLot(): Promise<Lot | null> {
    const product = products.find((item) => item.id === Number(productId));
    if (!product) {
      setMessage("Selecciona un producto del catálogo.");
      setError(true);
      return null;
    }
    try {
      const lot = await getFEFOLot(product.barcode);
      setLastLot(lot);
      return lot;
    } catch (reason) {
      setLastLot(null);
      setMessage(reason instanceof Error ? reason.message : "No hay un lote disponible.");
      setError(true);
      return null;
    }
  }

  async function addItem(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!Number.isInteger(quantity) || quantity < 1) {
      setMessage("La cantidad debe ser un número entero mayor que cero.");
      setError(true);
      return;
    }
    const lot = await findLot();
    if (!lot) return;
    const inCart = cart.filter((item) => item.barcode === lot.barcode).reduce((sum, item) => sum + item.quantity, 0);
    if (lot.available <= 0 || quantity > lot.available - inCart) {
      setMessage(`No hay stock disponible para ${lot.name}. Disponibles: ${Math.max(0, lot.available - inCart)}.`);
      setError(true);
      return;
    }
    const price = lot.discountApplied ? Math.round(lot.regularPrice * 0.7 * 100) / 100 : lot.regularPrice;
    setCart((items) => {
      const existing = items.find((item) => item.barcode === lot.barcode && item.price === price && item.discount === lot.discountApplied);
      if (existing) return items.map((item) => item === existing ? { ...item, quantity: item.quantity + quantity } : item);
      return [...items, {
        barcode: lot.barcode, name: lot.name, quantity, stock: lot.available,
        regularPrice: lot.regularPrice, price, discount: lot.discountApplied,
      }];
    });
    setQuantity(1);
    setMessage(`${quantity} unidad(es) de ${lot.name} agregadas${lot.discountApplied ? " con descuento por vencimiento" : ""}.`);
    setError(false);
  }

  async function checkout() {
    if (!cart.length || saving) {
      if (!cart.length) {
        setMessage("El carrito está vacío.");
        setError(true);
      }
      return;
    }
    setSaving(true);
    try {
      const result = await createSale(cart.map((item) => ({ barcode: item.barcode, quantity: item.quantity })));
      setMessage(`${result.message} · Venta #${result.saleId} · Total ${formatCurrency(result.totalPaid)}`);
      setError(false);
      setCart([]);
      setLastLot(null);
      notifyInventoryChanged();
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "No se pudo completar la venta; el inventario no fue modificado.");
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  function changeQuantity(target: CartItem, change: number) {
    const next = target.quantity + change;
    if (next < 1) {
      setCart((items) => items.filter((item) => item !== target));
      return;
    }
    const otherRows = cart.filter((item) => item.barcode === target.barcode && item !== target).reduce((sum, item) => sum + item.quantity, 0);
    if (next + otherRows > target.stock) {
      setMessage(`Stock disponible para ${target.name}: ${target.stock} unidades.`);
      setError(true);
      return;
    }
    setCart((items) => items.map((item) => item === target ? { ...item, quantity: next } : item));
  }

  return (
    <section className="page">
      <header className="page-heading">
        <div className="page-heading-mark">$</div>
        <div><p className="eyebrow">Caja</p><h1>Punto de venta</h1><p>Venta rápida con rotación FEFO y descuentos por vencimiento.</p></div>
      </header>
      {message && <p className={`message${error ? " message-error" : ""}`} role={error ? "alert" : "status"}>{message}</p>}
      <div className="pos-layout">
        <section className="card pos-main-card">
          <section className="product-entry">
            <div className="section-title"><span className="entry-step">01</span><div><p className="eyebrow">Nueva línea</p><h2>Agregar productos</h2></div></div>
            <form className="scan-form" onSubmit={addItem}>
              <div className="field">
                <label htmlFor="pos-product">Producto</label>
                <select id="pos-product" value={productId} onChange={(event) => setProductId(event.target.value)} required>
                  <option value="">{loading ? "Cargando productos…" : "Selecciona un producto"}</option>
                  {products.map((product) => <option key={product.id} value={product.id}>{product.name} · {product.category}</option>)}
                </select>
              </div>
              <div className="field quantity-field">
                <label htmlFor="pos-quantity">Cantidad</label>
                <input
                  id="pos-quantity"
                  type="number"
                  min="1"
                  step="1"
                  max={quantityLimit}
                  value={quantity}
                  disabled={!selectedProduct || availableForSelection <= 0}
                  onChange={(event) => {
                    const nextValue = Number(event.target.value);
                    if (!Number.isFinite(nextValue) || nextValue < 1) {
                      setQuantity(1);
                      return;
                    }
                    const maxQuantity = availableForSelection > 0 ? availableForSelection : 1;
                    setQuantity(Math.min(Math.max(1, nextValue), maxQuantity));
                  }}
                  required
                />
              </div>
              <button className="secondary-button" type="button" disabled={loading || !products.length || !selectedProduct} onClick={() => void findLot()}>Consultar stock</button>
              <button className="primary-button" type="submit" disabled={loading || !products.length || !canAddSelection}>Agregar al carrito <span>+</span></button>
            </form>
            <p className="form-hint">Selecciona un producto y una cantidad. Repite para agregar otros productos a la misma compra.</p>
            {lastLot && (
              <div className="lot-info">
                <span className="lot-info-icon">i</span>
                <p>
                  <strong>{lastLot.name} · {lastLot.number}</strong>
                  <span>
                    {availableForSelection > 0
                      ? `${availableForSelection} unidades disponibles · Vence ${formatDate(lastLot.expiresAt)}`
                      : `Sin stock disponible · Vence ${formatDate(lastLot.expiresAt)}`}
                  </span>
                </p>
              </div>
            )}
          </section>

          <section className="cart-section">
            <header className="cart-heading">
              <div className="section-title"><span className="entry-step">02</span><div><p className="eyebrow">Detalle de compra</p><h2>Carrito</h2></div></div>
              <span className="item-count">{cart.length} {cart.length === 1 ? "producto" : "productos"}</span>
            </header>
            {cart.length ? (
              <div className="table-wrap"><table className="cart-table">
                <thead><tr><th>Producto</th><th>Cant.</th><th>Precio unitario</th><th>Descuento FEFO</th><th className="amount-column">Subtotal</th><th><span className="visually-hidden">Acciones</span></th></tr></thead>
                <tbody>{cart.map((item, index) => (
                  <tr key={`${item.barcode}-${index}`}>
                    <td><strong>{item.name}</strong></td>
                    <td><div className="quantity-control" aria-label={`Cantidad de ${item.name}`}><button type="button" aria-label="Quitar una unidad" onClick={() => changeQuantity(item, -1)}>−</button><span>{item.quantity}</span><button type="button" aria-label="Agregar una unidad" disabled={item.quantity >= item.stock} onClick={() => changeQuantity(item, 1)}>+</button></div></td>
                    <td>{item.discount && <span className="old-price">{formatCurrency(item.regularPrice)}</span>}<strong>{formatCurrency(item.price)}</strong></td>
                    <td>{item.discount ? <span className="discount-badge">30% aplicado</span> : <span className="muted">—</span>}</td>
                    <td className="amount-column">{formatCurrency(item.price * item.quantity)}</td>
                    <td><button className="remove-item" type="button" aria-label={`Eliminar ${item.name}`} onClick={() => setCart((items) => items.filter((entry) => entry !== item))}>×</button></td>
                  </tr>
                ))}</tbody>
              </table></div>
            ) : <div className="empty-cart"><span className="empty-cart-mark">+</span><p>El carrito está vacío</p><span>Selecciona un producto para iniciar la venta.</span></div>}
          </section>
        </section>

        <aside className="card summary-card">
          <div className="summary-icon">$</div><p className="eyebrow">Resumen</p><h2>Venta actual</h2>
          <div className="summary-lines"><div><span>Subtotal</span><strong>{formatCurrency(subtotal)}</strong></div><div><span>Descuento total</span><strong className="discount-total">− {formatCurrency(discount)}</strong></div></div>
          <div className="summary-total"><span>Total a pagar</span><strong>{formatCurrency(total)}</strong></div>
          <button className="primary-button checkout-button" type="button" disabled={!cart.length || saving} onClick={() => void checkout()}>{saving ? "Procesando…" : "Procesar venta"} <span>→</span></button>
          <p className="checkout-note">El stock se actualiza al confirmar la venta.</p>
        </aside>
      </div>
    </section>
  );
}

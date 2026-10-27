"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { type FormEvent, type ReactNode, useState } from "react";
import { createProduct, notifyInventoryChanged } from "@/lib/api";

const navigation = [
  { href: "/resumen", label: "Resumen", icon: "▤" },
  { href: "/", label: "Inventario", icon: "▦" },
  { href: "/pos", label: "POS / Caja", icon: "▣" },
  { href: "/mermas", label: "Mermas", icon: "↘" },
];

export function DashboardShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState(false);

  async function submitProduct(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    setSaving(true);
    setMessage("");
    try {
      const product = await createProduct({
        name: String(values.get("name") ?? "").trim(),
        category: String(values.get("category") ?? "").trim(),
        regularPrice: Number(values.get("price")),
      });
      notifyInventoryChanged();
      setDialogOpen(false);
      form.reset();
      setMessage(`Producto ${product.name} creado. Código: ${product.barcode}.`);
      setError(false);
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "No se pudo crear el producto.");
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <Link className="brand" href="/" aria-label="Minimarket Control, inicio">
          <span className="brand-mark" aria-hidden="true"><span /></span>
          <span>Minimarket <strong>Control</strong></span>
        </Link>
        <div className="topbar-context"><span className="status-dot" />Sistema de gestión</div>
        <button className="primary-button create-product-button" onClick={() => setDialogOpen(true)} type="button">
          <span aria-hidden="true">+</span> Crear producto
        </button>
      </header>

      {message && (
        <div className={`toast${error ? " toast-error" : ""}`} role={error ? "alert" : "status"}>
          <span className="toast-indicator">{error ? "!" : "✓"}</span>
          <span>{message}</span>
          <button type="button" aria-label="Cerrar notificación" onClick={() => setMessage("")}>×</button>
        </div>
      )}

      <div className="dashboard-shell">
        <aside className="sidebar">
          <p className="sidebar-caption">MENÚ PRINCIPAL</p>
          <nav className="nav" aria-label="Navegación principal">
            {navigation.map((item) => {
              const active = pathname === item.href;
              return (
                <Link
                  className={active ? "active" : ""}
                  href={item.href}
                  key={item.href}
                  aria-current={active ? "page" : undefined}
                >
                  <span className="nav-icon" aria-hidden="true">{item.icon}</span>
                  <span>{item.label}</span>
                  <span className="nav-arrow" aria-hidden="true">›</span>
                </Link>
              );
            })}
          </nav>
          <div className="sidebar-note">
            <span className="sidebar-note-icon" aria-hidden="true">✓</span>
            <div><strong>Inventario al día</strong><span>Controla tus productos y ventas desde un solo lugar.</span></div>
          </div>
          <div className="sidebar-footer">MINIMARKET CONTROL <span>·</span> v1.0</div>
        </aside>
        <main className="content">{children}</main>
      </div>

      {dialogOpen && (
        <div className="modal-backdrop" onMouseDown={(event) => {
          if (event.target === event.currentTarget && !saving) setDialogOpen(false);
        }}>
          <section className="product-modal" role="dialog" aria-modal="true" aria-labelledby="product-modal-title">
            <header className="modal-heading">
              <div>
                <p className="eyebrow">Catálogo</p>
                <h2 id="product-modal-title">Crear producto nuevo</h2>
                <p>El código de barras se generará automáticamente.</p>
              </div>
              <button className="modal-close" type="button" aria-label="Cerrar" disabled={saving} onClick={() => setDialogOpen(false)}>×</button>
            </header>
            <form className="product-form" onSubmit={submitProduct}>
              <div className="field">
                <label htmlFor="product-name">Nombre</label>
                <input id="product-name" name="name" minLength={2} placeholder="Ej. Leche entera 1L" required />
              </div>
              <div className="field">
                <label htmlFor="product-category">Categoría</label>
                <input id="product-category" name="category" minLength={2} defaultValue="General" required />
              </div>
              <div className="field">
                <label htmlFor="product-price">Precio regular</label>
                <input id="product-price" name="price" type="number" min="0.01" step="0.01" required />
              </div>
              <footer className="modal-actions">
                <button className="secondary-button" type="button" disabled={saving} onClick={() => setDialogOpen(false)}>Cancelar</button>
                <button className="primary-button" type="submit" disabled={saving}>{saving ? "Guardando…" : "Guardar producto"}</button>
              </footer>
            </form>
          </section>
        </div>
      )}
    </div>
  );
}

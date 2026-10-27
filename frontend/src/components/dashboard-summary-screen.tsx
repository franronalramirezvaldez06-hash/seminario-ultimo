"use client";

import { useEffect, useState } from "react";
import { getDashboardSummary } from "@/lib/api";
import { formatCurrency, formatDate } from "@/lib/format";
import type { DashboardSummary } from "@/lib/types";

function formatWeekday(value: string): string {
  return new Intl.DateTimeFormat("es-CO", { weekday: "short" })
    .format(new Date(`${value}T12:00:00`))
    .replace(".", "");
}

function formatCompactCurrency(value: number): string {
  return new Intl.NumberFormat("es-CO", {
    style: "currency",
    currency: "COP",
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

export function DashboardSummaryScreen() {
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    void getDashboardSummary()
      .then((data) => {
        if (active) setSummary(data);
      })
      .catch((reason: unknown) => {
        if (active) setError(reason instanceof Error ? reason.message : "No se pudo cargar el resumen.");
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => { active = false; };
  }, []);

  function refreshSummary() {
    setLoading(true);
    setError("");
    void getDashboardSummary()
      .then(setSummary)
      .catch((reason: unknown) => {
        setError(reason instanceof Error ? reason.message : "No se pudo cargar el resumen.");
      })
      .finally(() => setLoading(false));
  }

  const sales = summary?.salesPerDay ?? [];
  const maxDailyTotal = Math.max(...sales.map((day) => day.totalPaid), 0);
  const weeklyTotal = sales.reduce((total, day) => total + day.totalPaid, 0);
  const weeklySales = sales.reduce((total, day) => total + day.saleCount, 0);

  return (
    <section className="page">
      <header className="page-heading summary-page-heading">
        <div>
          <p className="eyebrow">Vista general</p>
          <h1>Resumen del negocio</h1>
          <p>Revisa vencimientos y ventas recientes desde un solo lugar.</p>
        </div>
        <button className="secondary-button" type="button" onClick={refreshSummary} disabled={loading}>
          ↻ {loading ? "Actualizando…" : "Actualizar"}
        </button>
      </header>

      {error && (
        <div className="message message-error summary-error" role="alert">
          <span>{error}</span>
          <button className="text-button" type="button" onClick={refreshSummary} disabled={loading}>Reintentar</button>
        </div>
      )}

      <section className="summary-metrics" aria-label="Indicadores del negocio">
        <article className="summary-metric">
          <span className="metric-icon metric-products" aria-hidden="true">▦</span>
          <div><p>Productos registrados</p><strong>{summary?.totalProducts ?? "—"}</strong><span>Referencias en el catálogo</span></div>
        </article>
        <article className="summary-metric">
          <span className="metric-icon metric-expired" aria-hidden="true">!</span>
          <div><p>Productos vencidos</p><strong>{summary?.expiredProducts ?? "—"}</strong><span>{summary ? summary.expiredUnits : "—"} unidades en lotes vencidos</span></div>
        </article>
        <article className="summary-metric">
          <span className="metric-icon metric-expiring" aria-hidden="true">◷</span>
          <div><p>Por vencer · 7 días</p><strong>{summary?.expiringProducts ?? "—"}</strong><span>{summary ? summary.expiringUnits : "—"} unidades en esos lotes</span></div>
        </article>
        <article className="summary-metric">
          <span className="metric-icon metric-sales" aria-hidden="true">$</span>
          <div><p>Ventas de hoy</p><strong>{summary ? formatCurrency(summary.todaySalesTotal) : "—"}</strong><span>{summary ? summary.todaySaleCount : "—"} transacciones registradas</span></div>
        </article>
      </section>

      <section className="card sales-chart-card" aria-labelledby="daily-sales-title">
        <header className="sales-chart-heading">
          <div><p className="eyebrow">Actividad de caja</p><h2 id="daily-sales-title">Ventas por día</h2><p>Ingresos registrados durante los últimos 7 días.</p></div>
          <div className="sales-period-total"><strong>{summary ? formatCurrency(weeklyTotal) : "—"}</strong><span>{summary ? `${weeklySales} ventas en el período` : "Ventas de los últimos 7 días"}</span></div>
        </header>
        {loading && !summary ? (
          <p className="chart-state">Cargando datos de ventas…</p>
        ) : !summary ? (
          <p className="chart-state">No se pudieron cargar las ventas.</p>
        ) : sales.length ? (
          <div className="sales-chart-scroll">
            <div className="sales-chart" role="img" aria-label={`Ventas de los últimos siete días: ${weeklySales} transacciones por un total de ${formatCurrency(weeklyTotal)}.`}>
              {sales.map((day) => {
                const height = maxDailyTotal === 0 ? 0 : (day.totalPaid / maxDailyTotal) * 100;
                return (
                  <div className="sales-chart-column" key={day.date} title={`${formatDate(day.date)}: ${formatCurrency(day.totalPaid)} · ${day.saleCount} ventas`}>
                    <span className="sales-chart-value">{formatCompactCurrency(day.totalPaid)}</span>
                    <div className="sales-chart-track">
                      <div className={`sales-chart-bar${day.totalPaid === 0 ? " sales-chart-bar-empty" : ""}`} style={{ height: `${Math.max(height, day.totalPaid > 0 ? 4 : 0)}%` }} />
                    </div>
                    <span className="sales-chart-weekday">{formatWeekday(day.date)}</span>
                    <span className="sales-chart-date">{formatDate(day.date)}</span>
                    <span className="sales-chart-count">{day.saleCount} {day.saleCount === 1 ? "venta" : "ventas"}</span>
                  </div>
                );
              })}
            </div>
          </div>
        ) : (
          <p className="chart-state">Aún no hay ventas registradas para mostrar.</p>
        )}
        <footer className="sales-chart-legend"><span aria-hidden="true" /> Total vendido por día <span>El número de ventas aparece bajo cada fecha.</span></footer>
      </section>

      <p className="summary-note">Los productos vencidos y por vencer se calculan sobre lotes con existencias. “Por vencer” incluye los próximos 7 días.</p>
    </section>
  );
}

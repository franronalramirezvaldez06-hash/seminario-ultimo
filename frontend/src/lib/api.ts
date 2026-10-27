import type {
  ExpiringLot,
  DashboardSummary,
  InventoryProduct,
  Lot,
  Product,
  SaleResult,
  WasteEntry,
  WasteReason,
} from "./types";

const apiBase = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api").replace(/\/$/, "");

interface ApiProduct {
  id: number;
  codigo_barras: string;
  nombre: string;
  categoria?: string;
  precio_regular: number | string;
}

interface ApiLot {
  id: number;
  producto_id: number;
  codigo_barras: string;
  numero_lote: string;
  cantidad_disponible: number;
  fecha_vencimiento: string;
  nombre: string;
  categoria: string;
  precio_regular: number | string;
  dias_hasta_vencimiento: number;
  descuento_aplicado: boolean;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
    cache: "no-store",
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null) as { detail?: string } | null;
    throw new Error(body?.detail ?? `La solicitud falló (${response.status}).`);
  }
  return response.json() as Promise<T>;
}

function mapProduct(product: ApiProduct): Product {
  return {
    id: product.id,
    barcode: product.codigo_barras,
    name: product.nombre,
    category: product.categoria ?? "General",
    regularPrice: Number(product.precio_regular),
  };
}

export async function getProducts(): Promise<Product[]> {
  const products = await request<ApiProduct[]>("/productos");
  return products.map(mapProduct).sort((a, b) => a.name.localeCompare(b.name));
}

export async function getInventory(): Promise<InventoryProduct[]> {
  const rows = await request<Array<ApiProduct & {
    producto_id: number;
    stock_disponible: number;
    lotes_activos: number;
    proximo_vencimiento: string | null;
  }>>("/inventario");
  return rows.map((row) => ({
    ...mapProduct({ ...row, id: row.producto_id }),
    available: row.stock_disponible,
    activeLots: row.lotes_activos,
    nextExpiry: row.proximo_vencimiento,
  }));
}

export async function getDashboardSummary(): Promise<DashboardSummary> {
  const result = await request<{
    productos_total: number;
    productos_vencidos: number;
    unidades_vencidas: number;
    productos_por_vencer: number;
    unidades_por_vencer: number;
    ventas_hoy: number;
    total_ventas_hoy: number | string;
    ventas_por_dia: Array<{
      fecha: string;
      cantidad_ventas: number;
      total_ventas: number | string;
    }>;
  }>("/resumen");
  return {
    totalProducts: result.productos_total,
    expiredProducts: result.productos_vencidos,
    expiredUnits: result.unidades_vencidas,
    expiringProducts: result.productos_por_vencer,
    expiringUnits: result.unidades_por_vencer,
    todaySaleCount: result.ventas_hoy,
    todaySalesTotal: Number(result.total_ventas_hoy),
    salesPerDay: result.ventas_por_dia.map((day) => ({
      date: day.fecha,
      saleCount: day.cantidad_ventas,
      totalPaid: Number(day.total_ventas),
    })),
  };
}

export async function createProduct(input: {
  name: string;
  category: string;
  regularPrice: number;
}): Promise<Product> {
  const product = await request<ApiProduct>("/productos", {
    method: "POST",
    body: JSON.stringify({
      nombre: input.name,
      categoria: input.category,
      precio_regular: input.regularPrice,
    }),
  });
  return mapProduct(product);
}

export function receiveLot(input: {
  productId: number;
  quantity: number;
  expiresAt: string;
}): Promise<unknown> {
  const today = new Date();
  const expiry = new Date(`${input.expiresAt}T00:00:00`);
  today.setHours(0, 0, 0, 0);
  const daysToExpiry = Math.max(0, Math.ceil((expiry.getTime() - today.getTime()) / 86_400_000));
  return request("/lotes/ingresar", {
    method: "POST",
    body: JSON.stringify({
      producto_id: input.productId,
      cantidad: input.quantity,
      dias_para_vencer: daysToExpiry,
      fecha_vencimiento: input.expiresAt,
    }),
  });
}

export async function getFEFOLot(barcode: string): Promise<Lot> {
  const lot = await request<ApiLot>(`/lotes/peps/${encodeURIComponent(barcode)}`);
  return {
    id: lot.id,
    productId: lot.producto_id,
    barcode: lot.codigo_barras,
    number: lot.numero_lote,
    available: lot.cantidad_disponible,
    expiresAt: lot.fecha_vencimiento,
    name: lot.nombre,
    category: lot.categoria,
    regularPrice: Number(lot.precio_regular),
    daysUntilExpiry: lot.dias_hasta_vencimiento,
    discountApplied: lot.descuento_aplicado,
  };
}

export async function createSale(items: Array<{ barcode: string; quantity: number }>): Promise<SaleResult> {
  const result = await request<{
    mensaje: string;
    venta_id: number;
    total_pagado: number | string;
    subtotal: number | string;
    descuento_total: number | string;
  }>("/ventas", {
    method: "POST",
    body: JSON.stringify(items.map((item) => ({
      codigo_barras: item.barcode,
      cantidad: item.quantity,
    }))),
  });
  return {
    message: result.mensaje,
    saleId: result.venta_id,
    totalPaid: Number(result.total_pagado),
    subtotal: Number(result.subtotal),
    totalDiscount: Number(result.descuento_total),
  };
}

export async function getWaste(): Promise<WasteEntry[]> {
  const rows = await request<Array<{
    id: number;
    lote_id: number;
    cantidad_retirada: number;
    motivo: WasteReason;
    fecha_registro: string;
    nombre_producto: string;
    numero_lote: string;
  }>>("/mermas");
  return rows.map((row) => ({
    id: row.id,
    lotId: row.lote_id,
    quantity: row.cantidad_retirada,
    reason: row.motivo,
    recordedAt: row.fecha_registro,
    productName: row.nombre_producto,
    lotNumber: row.numero_lote,
  }));
}

export async function createWaste(input: {
  lotId: number;
  quantity: number;
  reason: WasteReason;
}): Promise<void> {
  await request("/mermas", {
    method: "POST",
    body: JSON.stringify({
      lote_id: input.lotId,
      cantidad_retirada: input.quantity,
      motivo: input.reason,
    }),
  });
}

export async function getExpiringLots(days = 7): Promise<ExpiringLot[]> {
  const rows = await request<Array<{
    lote_id: number;
    producto_id: number;
    codigo_barras: string;
    nombre_producto: string;
    categoria: string;
    numero_lote: string;
    cantidad_disponible: number;
    fecha_vencimiento: string;
    dias_para_vencer: number;
  }>>(`/lotes/proximos-vencer?dias=${days}`);
  return rows.map((row) => ({
    lotId: row.lote_id,
    productId: row.producto_id,
    barcode: row.codigo_barras,
    productName: row.nombre_producto,
    category: row.categoria,
    lotNumber: row.numero_lote,
    available: row.cantidad_disponible,
    expiresAt: row.fecha_vencimiento,
    daysToExpiry: row.dias_para_vencer,
  }));
}

export function notifyInventoryChanged(): void {
  window.dispatchEvent(new Event("inventory-updated"));
}

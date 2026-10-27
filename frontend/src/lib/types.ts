export interface Product {
  id: number;
  barcode: string;
  name: string;
  category: string;
  regularPrice: number;
}

export interface InventoryProduct extends Product {
  available: number;
  activeLots: number;
  nextExpiry: string | null;
}

export interface Lot {
  id: number;
  productId: number;
  barcode: string;
  number: string;
  available: number;
  expiresAt: string;
  name: string;
  category: string;
  regularPrice: number;
  daysUntilExpiry: number;
  discountApplied: boolean;
}

export interface ExpiringLot {
  lotId: number;
  productId: number;
  barcode: string;
  productName: string;
  category: string;
  lotNumber: string;
  available: number;
  expiresAt: string;
  daysToExpiry: number;
}

export interface WasteEntry {
  id: number;
  lotId: number;
  quantity: number;
  reason: "VENCIDO" | "DANADO" | "ROTO";
  recordedAt: string;
  productName: string;
  lotNumber: string;
}

export type WasteReason = WasteEntry["reason"];

export interface SaleResult {
  message: string;
  saleId: number;
  totalPaid: number;
  subtotal: number;
  totalDiscount: number;
}

export interface DashboardSummary {
  totalProducts: number;
  expiredProducts: number;
  expiredUnits: number;
  expiringProducts: number;
  expiringUnits: number;
  todaySaleCount: number;
  todaySalesTotal: number;
  salesPerDay: Array<{
    date: string;
    saleCount: number;
    totalPaid: number;
  }>;
}

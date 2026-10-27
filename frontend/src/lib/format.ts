export function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("es-CO", {
    style: "currency",
    currency: "COP",
    maximumFractionDigits: 2,
  }).format(amount);
}

export function formatDate(value: string): string {
  const date = value.slice(0, 10);
  return new Intl.DateTimeFormat("es-CO", { dateStyle: "medium" }).format(new Date(`${date}T12:00:00`));
}

export function daysLabel(days: number): string {
  if (days < 0) return `Vencido hace ${Math.abs(days)} día(s)`;
  if (days === 0) return "Vence hoy";
  return `Vence en ${days} día(s)`;
}

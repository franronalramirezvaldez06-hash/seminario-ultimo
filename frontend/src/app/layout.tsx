import type { Metadata } from "next";
import type { ReactNode } from "react";
import { DashboardShell } from "@/components/dashboard-shell";
import "./globals.css";

export const metadata: Metadata = {
  title: "Minimarket Control",
  description: "Inventario, ventas y control de mermas para tu minimarket.",
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="es">
      <body><DashboardShell>{children}</DashboardShell></body>
    </html>
  );
}

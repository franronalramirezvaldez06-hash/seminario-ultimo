# Minimarket Control — migración Next.js + Go

Migración del sistema que estaba en `Nueva carpeta`. El proyecto original permanece intacto.

## Estructura

- `frontend/`: Next.js App Router, React y TypeScript. Las pantallas de inventario, caja y mermas están separadas en `src/components/`, y comparten el cliente HTTP tipado de `src/lib/`.
- `backend/`: API REST en Go. `cmd/server/` inicia el servicio, `internal/api/` contiene rutas y reglas de negocio, e `internal/store/` gestiona el almacenamiento.
- `backend/data/store.json`: copia independiente del estado del minimarket al momento de la migración. Los cambios en esta copia no modifican el proyecto original.

## Requisitos

- Node.js 20.9 o posterior y npm.
- Go 1.24 o posterior.

## Iniciar en Windows

Abre dos terminales desde esta carpeta.

Si PowerShell no reconoce `go` o `npm`, agrega las instalaciones de este equipo al `PATH` de cada terminal:

```powershell
$env:PATH = 'C:\Program Files\Go\bin;C:\Program Files\nodejs;' + $env:PATH
```

Terminal 1 — API:

```powershell
cd backend
go run ./cmd/server
```

La API estará disponible en `http://localhost:8080`; su comprobación de salud es `http://localhost:8080/api/health`.

Terminal 2 — interfaz:

```powershell
cd frontend
npm install
npm run dev
```

Abre `http://localhost:3000`. Si la API está en otra dirección, configura `NEXT_PUBLIC_API_URL` antes de iniciar Next.js (por ejemplo, `http://localhost:8080/api`).

## Iniciar con Docker

Instala e inicia Docker Desktop y, desde esta carpeta, ejecuta:

```powershell
docker compose up --build -d
docker compose ps
```

Cuando el backend indique `healthy`, abre `http://localhost:3000`. La API queda disponible en `http://localhost:8080`. Compose espera a que el backend pase su comprobación de salud antes de iniciar el frontend.

Los datos se conservan en `backend/data/store.json` mediante un volumen local; detener o reconstruir los contenedores no los elimina. Para ver los registros utiliza `docker compose logs -f`; para apagar ambos servicios ejecuta `docker compose down`.

El frontend establece `NEXT_PUBLIC_API_URL` al construirse la imagen. Si accedes desde otro equipo de la red, reemplaza el host `localhost` por la dirección del equipo que ejecuta Docker al construir:

```powershell
$env:NEXT_PUBLIC_API_URL = 'http://192.168.1.10:8080/api'
docker compose up --build -d
```

## Verificación

```powershell
cd backend
go test ./...
```

```powershell
cd frontend
npm run typecheck
npm run lint
npm run build
```

## Operación migrada

- Crear productos con código de barras generado automáticamente.
- Consultar existencias por producto y registrar lotes con fecha de vencimiento.
- Aplicar descuento del 30% cuando el lote FEFO vence en cinco días o menos.
- Procesar ventas y retirar existencias del lote que vence primero.
- Registrar mermas (vencido, dañado o roto) y consultar su historial.
- Consultar lotes vencidos o próximos a vencer en los siguientes siete días.

Los datos se guardan en `backend/data/store.json`. Para cambiar la ubicación, establece `DATA_FILE`; para cambiar el puerto de escucha, establece `API_ADDRESS` (por defecto `:8080`).

# seminario-ultimo

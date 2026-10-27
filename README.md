# Minimarket Control — migración Next.js + Go

Migración del sistema que estaba en `Nueva carpeta`. El proyecto original permanece intacto.

## Estructura

- `frontend/`: Next.js App Router, React y TypeScript. Las pantallas de inventario, caja y mermas están separadas en `src/components/`, y comparten el cliente HTTP tipado de `src/lib/`.
- `backend/`: API REST en Go. `cmd/server/` inicia el servicio, `internal/api/` contiene rutas y reglas de negocio, e `internal/store/` gestiona el almacenamiento.
- Los datos del minimarket se guardan en PostgreSQL, en el esquema `minimarket`.

## Requisitos

- Node.js 20.9 o posterior y npm.
- Go 1.24 o posterior.
- PostgreSQL ejecutándose en Docker y accesible por el puerto `5432`.

## Iniciar en Windows

El backend crea automáticamente el esquema `minimarket` y sus tablas al iniciar. Se conecta al PostgreSQL local con el usuario `postgres` y la base de datos `postgres` por defecto. Configura la contraseña del contenedor antes de arrancar la API:

```powershell
$env:POSTGRES_PASSWORD = 'tu-contraseña-de-PostgreSQL'
```

Si tu contenedor utiliza otros valores, también puedes configurar `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER` y `POSTGRES_DB`.

Terminal 1 — API:

```powershell
cd backend
go run ./cmd/server
```

Si PowerShell no reconoce `go` o `npm`, agrega las instalaciones de este equipo al `PATH` de cada terminal:

```powershell
$env:PATH = 'C:\Program Files\Go\bin;C:\Program Files\nodejs;' + $env:PATH
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
$env:POSTGRES_PASSWORD = 'tu-contraseña-de-PostgreSQL'
docker compose up --build -d
docker compose ps
```

Compose conecta el backend al PostgreSQL ya publicado en el puerto `5432` del equipo y crea las tablas automáticamente. Cuando el backend indique `healthy`, abre `http://localhost:3000`. La API queda disponible en `http://localhost:8080`. Compose espera a que el backend pase su comprobación de salud antes de iniciar el frontend.

El inventario, los lotes, las ventas y las mermas se guardan en PostgreSQL. Para ver los registros utiliza `docker compose logs -f`; para apagar ambos servicios ejecuta `docker compose down`.

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
- Consultar un resumen con productos vencidos, próximos a vencer y ventas diarias de los últimos siete días.
- Consultar existencias por producto y registrar lotes con fecha de vencimiento.
- Aplicar descuento del 30% cuando el lote FEFO vence en cinco días o menos.
- Procesar ventas y retirar existencias del lote que vence primero.
- Registrar mermas (vencido, dañado o roto) y consultar su historial.
- Consultar lotes vencidos o próximos a vencer en los siguientes siete días.

Los datos se guardan en las tablas del esquema `minimarket` de PostgreSQL. Para cambiar el puerto de escucha, establece `API_ADDRESS` (por defecto `:8080`).

# seminario-ultimo

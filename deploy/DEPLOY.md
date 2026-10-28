# Deploy

`modos-cpu` se distribuye como un único binario Go que embebe el frontend Vue. El binario escucha por defecto en `:9080`; la imagen Docker y la unidad systemd lo arrancan en `:8080`. Necesita sólo conectividad de red entrante para HTTP+WebSocket y un archivo SQLite persistente.

Tres formas razonables de operarlo, ordenadas por simplicidad.

## Opción A — Docker

`deploy/Dockerfile` es un multistage que compila el frontend con Node 24, compila el backend con Go 1.26 (`CGO_ENABLED=0`) y produce una imagen final basada en `gcr.io/distroless/static-debian12:nonroot`. Tamaño final ~10 MB.

```bash
# Build
docker build -t modos-cpu:latest -f deploy/Dockerfile .

# Smoke test local
docker run --rm -p 8080:8080 modos-cpu:latest
curl -s http://127.0.0.1:8080/api/health    # {"ok":true,...}

# Producción (memoria/CPU limitadas, base de datos en un volumen)
docker run -d \
  --name modos-cpu \
  --restart=unless-stopped \
  -p 8080:8080 \
  -v modos-data:/home/nonroot \
  --memory=128m \
  --cpus=0.5 \
  modos-cpu:latest
```

### Habilitar el admin

El panel `/admin` está deshabilitado salvo que se definan las tres env vars. Para generar los valores localmente:

```bash
make admin-hash PASS='your-password'   # imprime el hash bcrypt
make session-secret                    # 32 bytes hex
```

Y pasarlos al `docker run`:

```bash
docker run -d \
  --name modos-cpu \
  --restart=unless-stopped \
  -p 8080:8080 \
  -e ADMIN_USER='admin' \
  -e ADMIN_PASS_HASH='<bcrypt-hash>' \
  -e SESSION_SECRET='<hex>' \
  modos-cpu:latest
```

La base SQLite vive en `/home/nonroot/.local/state/modos-cpu/state.db` dentro del contenedor; el volumen `modos-data` la conserva entre recreaciones. Para usar otra ruta, montar un directorio escribible por el UID 65532 y definir `MODOS_STATE_DB`.

Detrás de un dominio real definir también `ALLOWED_ORIGINS` (hosts permitidos para el WebSocket) y `TRUSTED_PROXY_HEADER` (cabecera con la IP real del cliente que pone el proxy: `CF-Connecting-IP` detrás de Cloudflare, `X-Real-IP` con el `nginx.conf` de ejemplo). Sin esa variable las cabeceras reenviadas se ignoran y los límites de intentos usan la IP de la conexión.

Variables opcionales: `MAX_PUBLIC_SESSIONS` (default 10), `MAX_PRIVILEGED_SESSIONS` (default 5), `SESSION_TTL` (default 30m), `INVITE_TTL` (default 24h), `CLEANUP_INTERVAL` (default 60s), `AUTH_LOGIN_LIMIT` (intentos de login de estudiantes por IP cada 15 minutos, default 5; el login del admin queda fijo en 5), `EMBEDDER_MAX_CONCURRENCY` (consultas simultáneas de ARCHIUS al embedder Jina, default 2).

## Opción B — Binario + systemd

Compilar estático y servir tras un usuario sin privilegios.

```bash
# Build estático
cd web && npm run build && cd ..
CGO_ENABLED=0 go build -ldflags='-s -w' -o bin/modos-cpu ./cmd/modos-cpu

# Copiar binario y unit al servidor
scp bin/modos-cpu user@server:/tmp/
scp deploy/modos-cpu.service user@server:/tmp/
```

En el servidor:

```bash
sudo useradd -r -s /sbin/nologin modos
sudo mkdir -p /opt/modos-cpu
sudo mv /tmp/modos-cpu /opt/modos-cpu/
sudo chown -R modos:modos /opt/modos-cpu
sudo chmod +x /opt/modos-cpu/modos-cpu
sudo mv /tmp/modos-cpu.service /etc/systemd/system/

# (Opcional) Crear /opt/modos-cpu/.env (ver .env.sample) con las variables
# del admin, ALLOWED_ORIGINS y TRUSTED_PROXY_HEADER. El servidor lo lee al
# arrancar. La base queda en /var/lib/modos-cpu (StateDirectory de la unit).

sudo systemctl daemon-reload
sudo systemctl enable --now modos-cpu
systemctl status modos-cpu
curl http://127.0.0.1:8080/api/health
```

## Opción C — Reverse proxy

`deploy/nginx.conf` tiene un server block listo con WebSocket forwarding. Ajustar `server_name` al hostname destino y apuntar `proxy_pass` al backend. El bloque reenvía la IP del cliente en `X-Real-IP`, así que el backend debe arrancar con `TRUSTED_PROXY_HEADER=X-Real-IP` y escuchar solo en `127.0.0.1`.

```bash
sudo cp deploy/nginx.conf /etc/nginx/sites-available/modos-cpu
sudo ln -s /etc/nginx/sites-available/modos-cpu /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

Para HTTPS, combinar con Let's Encrypt (certbot), un reverse proxy con TLS termination (Caddy, Traefik) o un tunnel externo (Cloudflare Tunnel, Tailscale Funnel). El backend nunca termina TLS por sí mismo.

## Verificación end-to-end

```bash
curl -s http://<host>:8080/api/health        # {"ok":true,...}
curl -s http://<host>:8080/api/programs | jq 'length'   # número de demos cargados
```

Después abrir el frontend en el navegador y correr el demo `counter` desde el ProgramPicker. Si los registros se mueven en vivo, la pipeline simulador → WebSocket → UI está funcionando.

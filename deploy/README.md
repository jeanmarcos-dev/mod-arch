# Deploy assets

Archivos para correr `modos-cpu` en un servidor.

| Archivo | Uso |
|---|---|
| `Dockerfile` | Multistage build: frontend Vue + backend Go → imagen distroless ~10 MB. |
| `modos-cpu.service` | Unit de systemd; corre el binario bajo el usuario `modos` con hardening básico. |
| `nginx.conf` | Reverse proxy con WebSocket forwarding; ajustar `server_name`. |
| `DEPLOY.md` | Guía paso a paso (Docker / binario+systemd / nginx). |

El binario es estático (`CGO_ENABLED=0`), no depende de glibc y embebe el frontend, así que basta con dejarlo en cualquier máquina Linux con acceso al puerto que se quiera exponer. Para HTTPS terminar el TLS antes (Caddy, Traefik, Cloudflare Tunnel, etc.).

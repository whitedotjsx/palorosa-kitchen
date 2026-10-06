# cf-quick-tunnel-rs (vendored + parcheado)

> **Vendored in palorosa-kitchen.** This copy lives at
> `apps/desktop/third_party/cf-quick-tunnel-rs`. It adds `[lib] crate-type`
> and `src/ffi.rs` (a small C ABI over the named-tunnel manager) so the Go
> desktop app can run the connector in process through
> `apps/desktop/internal/cftunnel` instead of the cloudflared binary (D32).
> `scripts/build-desktop.ps1` builds it as `cf-tunnel.dll` with
> `RUSTFLAGS=-C target-feature=+crt-static` and embeds it in the exe.

Tunnels de Cloudflare sin el binario `cloudflared`: **quick tunnels**
(`https://*.trycloudflare.com`) y ahora **named tunnels** (hostname propio).
Crate vendeareada de `cloudflare-quick-tunnel` 0.3.1 con parches de
fiabilidad, soporte named, y un cliente CLI con perfil release liviano.

## Por qué

`cloudflared` pesa ~38-50 MB (Go). Esta ruta habla QUIC + Cap'n Proto-RPC
contra el edge de Cloudflare (`argotunnel`) de forma nativa: binario release
**3.58 MB** (era 4.4 MB antes de la dieta de tamaño), cero subprocess.

## Estructura

- `src/` — crate vendeareada y parcheada (`cloudflare-quick-tunnel 0.3.1`).
- `client/` — CLI `qtt-client` (usa el crate por path).

## Parches aplicados sobre 0.3.1

1. **`src/pool.rs` — liveness probe (fix de los 502 intermitentes).**
   El pool devolvía sockets keep-alive que el origen ya había cerrado
   (Python `http.server` es HTTP/1.0 y cierra tras cada respuesta). La
   siguiente petición sobre ese socket muerto daba 502. Ahora, antes de
   reusar un socket, se hace un `peek` no bloqueante con timeout de 5 ms:
   `Ok(0)` (FIN del peer) o `Ok(n>0)` (bytes inesperados) → se descarta y se
   abre uno nuevo. Timeout → saludable, se reusa. Idealmente los origins
   deberían usar HTTP/1.1 keep-alive; el probe cubre el caso contrario.

2. **`src/proxy.rs` — fix de HEAD.**
   Un `HEAD` tiene `Content-Length` en la respuesta pero no body. La ruta
   pooled intentaba leer `Content-Length` bytes y bloqueaba → 502. Ahora si
   el método es HEAD se manda la cabecera y se cierra sin leer body (caso
   legítimo, no error).

3. **`src/proxy.rs` — decodifica las respuestas `Transfer-Encoding: chunked`.**
   El edge re-enmarca el body para el cliente (igual que `cloudflared`, que
   reenvía el body ya decodificado por `net/http`), así que reenviar el
   framing del origen incrustaba las líneas de tamaño de chunk en el cuerpo
   (`77a7b\r\n<!doctype html>…`) y, con un origen keep-alive, el fin de la
   respuesta nunca se señalaba (la conexión quedaba colgada). Ahora
   `run_pooled` y `run_bidi` decodifican el chunked antes de escribir al
   edge y se elimina la cabecera hop-by-hop `Transfer-Encoding` de la
   metadata. Síntoma que arregla: el panel del host servido por Go (>2 KB
   por respuesta, sin `Content-Length`) llegaba ilegible al espectador y la
   ventana abría en blanco.

## Dieta de tamaño (2026-10-05)

De 4.41 MB a 3.59 MB (−18.7%), sin perder funciones ni tests (44/44 verdes).

1. **`reqwest` fuera.** Solo se usaba para el `POST /tunnel` inicial, y
   arrastraba hyper + h2 + tower + su stack de TLS. Reemplazado por
   `src/http.rs`: un cliente HTTP/1.1 mínimo sobre tokio + rustls (el mismo
   rustls que ya usa el crate para QUIC y orígenes HTTPS). Parsea con
   `httparse` (ya era dependencia directa), soporta `Content-Length` y
   chunked, `Connection: close`. Config TLS cacheada en un `OnceLock`, y el
   crypto provider de ring se instala explícitamente (antes lo traía reqwest).
2. **`build-std` con `optimize_for_size`.** Build con nightly:
   `cargo +nightly build --release -Z build-std=std,panic_abort -Z build-std-features=optimize_for_size`.
   Compila el `std` propio con `opt-level=z` en vez del precompilado.
   (El perfil release del cliente ya usa `opt-level="z"`, `lto="fat"`,
   `codegen-units=1`, `panic=abort`, `strip`.)

Otras palancas no aplicadas: quitar `hickory-resolver` (DNS-over-TLS) y usar
el resolver del sistema, y recortar `uuid` a la generación v4. Se dejaron
porque el ahorro es menor que el costo en robustez/DNS.

## Named tunnels (nuevo)

Mismo transporte QUIC + capnp-RPC, pero la credencial viene del token o
credentials file en vez del POST anónimo, y el routing es por hostname.

### Cambios en el core

- **`credentials.rs`** — parsea el token del dashboard (base64 de
  `{"a":account,"t":tunnel-uuid,"s":secret}`) y el credentials file
  (`AccountTag`/`TunnelID`/`TunnelSecret`). `parse_auto` detecta cualquiera.
- **`router.rs`** — tabla de routing hostname → servicio local, reemplaza el
  puerto fijo de quick. El pool ahora es por-dirección (soporta varios origins).
- **`ingress.rs`** — parsea la lista `ingress` (YAML o JSON, mismo parser)
  del config local o del que empuja el edge.
- **`rpc_config.rs`** — implementa `ConfigurationManager.updateConfiguration`:
  el edge empuja la config de los named remotely-managed y hacemos hot-swap
  del router. El binder heredado (stub que la rechazaba) ya no aplica.
- **`origin_tls.rs`** — TLS al origen para reglas `https://...`.
- **`manager.rs`** — `NamedTunnelManager` + `QuickTunnelManager`; HA default 4
  en named (igual que cloudflared), 2 en quick.

### Uso

```
cd client
cargo build --release

# quick
./target/release/qtt-client quick 8080 [--ha N]

# named (token del dashboard)
./target/release/qtt-client named --token <TUNNEL_TOKEN> [--ha N]

# named (credentials file + ingress local)
./target/release/qtt-client named --credentials <id>.json --ingress config.yml
```

Ingress (YAML o JSON):

```yaml
ingress:
  - hostname: app.example.com
    service: http://127.0.0.1:8081
    originRequest:
      httpHostHeader: internal.local
  - service: http_status:404
```

Si no pasás `--ingress`, se asume named **remotely-managed**: el edge empuja
la config por `updateConfiguration` y el router se actualiza solo.

### Validación

- 40/40 tests unit/adapter en verde (`cargo test`), incluidos parseo de
  token/credentials, routing por hostname, ingress YAML/JSON, y el liveness
  probe del pool.
- Quick tunnel re-verificado en vivo tras el refactor: 10/10 GET (sin 502).
- El camino named end-to-end queda pendiente de probar contra el edge con un
  token real (requiere una cuenta Cloudflare); el transporte es el mismo que
  el de quick, que sí está verificado en vivo.

## Cliente

```
cd client
cargo build --release
./target/release/qtt-client quick <puerto_local> [--ha N]
```

- `--ha` (default 4): conexiones QUIC paralelas al edge; enmascara caídas
  de un POP. Máximo 4 (límite que también usa cloudflared).
- Perfil release: `opt-level="z"`, `lto="fat"`, `codegen-units=1`,
  `panic="abort"`, `strip="symbols"` → 4.1 MB.

## Resultados medidos (2026-10-04, VPS Linux)

| Prueba | Antes (0.3.1 stock) | Después (parcheado, HA=4) |
| --- | --- | --- |
| 40 GET secuenciales | ~40% 502 | 40/40 = 200 |
| 10 HEAD | 0/10 (siempre 502) | 10/10 = 200 |
| 80 GET concurrentes (x8) | — | 80/80 = 200 |
| 404 / archivos | — | correcto (404/200) |
| Peso binario release | 112 MB (debug) | 4.1 MB |
| RSS en reposo | — | ~8 MB |

## Limitaciones

- El camino named está implementado y testeado a nivel de código, pero no
  verificado contra el edge con una cuenta real (no tengo token de Cloudflare).
- El protocolo del edge no está documentado y puede cambiar upstream.
- Requiere salida UDP hacia el puerto 7844 del edge (QUIC).
- Ingress soporta el subconjunto HTTP: `hostname`, `service` (http/https),
  `originRequest.httpHostHeader`. No WARP, UDP, ni Argo Smart Routing.

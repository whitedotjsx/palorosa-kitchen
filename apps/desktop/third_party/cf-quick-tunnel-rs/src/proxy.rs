//! Per-request HTTP/1.1 proxy: bridge an inbound capnp-framed
//! stream from the edge to the local origin the caller wants to
//! expose.
//!
//! Two code paths share the same entry point:
//!
//! - **Pooled HTTP/1.1** — for plain requests where both sides
//!   advertise `Content-Length` and no Upgrade / chunked / Connection:
//!   close. Acquires a keep-alive socket from [`crate::pool::Pool`],
//!   forwards Content-Length-bounded request and response bodies,
//!   releases the socket back to the pool. Slashes per-request
//!   socket connect cost. Only plain (non-TLS) origins are pooled.
//!
//! - **Bidi-pump fallback** — WebSocket Upgrades (101 Switching
//!   Protocols), Transfer-Encoding: chunked, close-bound responses,
//!   and all TLS origins run two concurrent byte pumps until either
//!   half closes. The socket is dropped at the end; no pooling.
//!
//! The origin is chosen by [`crate::router::Router`] from the request
//! host: quick tunnels always resolve to one fixed port; named
//! tunnels map hostnames to local services.

use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

use futures::{AsyncReadExt, AsyncWriteExt};
use tokio::io::{AsyncRead, AsyncReadExt as _, AsyncWrite, AsyncWriteExt as _};
use tokio::net::TcpStream;
use tracing::{debug, warn};

use crate::error::TunnelError;
use crate::pool::Pool;
use crate::router::Router;
use crate::stream::{
    self, ConnectRequest, ConnectionType, HTTP_HEADER_KEY, HTTP_HOST_KEY, HTTP_METHOD_KEY,
    HTTP_STATUS_KEY,
};

/// Byte counters the supervisor accumulates across all streams.
#[derive(Debug, Default, Clone)]
pub struct StreamCounters {
    pub bytes_in: Arc<AtomicU64>,
    pub bytes_out: Arc<AtomicU64>,
}

/// How long we wait for the local TCP listener to accept the first
/// byte. Origins often run alongside a process that just finished
/// booting; 5s gives a generous margin.
pub const LOCAL_CONNECT_TIMEOUT: Duration = Duration::from_secs(5);

/// Hard cap on the response header section.
const MAX_HEADER_BYTES: usize = 32 * 1024;

/// Connect options resolved from the router for one request.
#[derive(Debug, Clone)]
pub struct OriginTarget {
    pub addr: String,
    pub tls: bool,
    pub host_header: Option<String>,
}

/// Drive one inbound request stream to completion. Reads the
/// `ConnectRequest`, resolves the origin via `router`, dispatches by
/// type, writes the `ConnectResponse` back, pumps the body.
pub async fn handle_inbound_stream(
    router: Arc<Router>,
    send: quinn::SendStream,
    recv: quinn::RecvStream,
    counters: StreamCounters,
    pool: Arc<Pool>,
) -> Result<(), TunnelError> {
    let (mut reader, mut writer) = stream::split(send, recv);
    let req = stream::read_connect_request(&mut reader).await?;
    debug!(dest = %req.dest, ty = ?req.conn_type, "inbound stream");

    let tcp = match req.conn_type {
        ConnectionType::Http | ConnectionType::Websocket => {
            proxy_http(&router, req, reader, writer, counters, pool).await
        }
        ConnectionType::Tcp => proxy_tcp(&router, &req, &mut reader, &mut writer, &counters).await,
    };
    if let Err(e) = &tcp {
        warn!(error = %e, "stream proxy failed");
    }
    tcp
}

// ── Request shape analysis ───────────────────────────────────────────────────

#[derive(Debug, Clone, Copy)]
struct RequestShape {
    /// Either a known body length (`Some(n)`) or `None` when the
    /// request advertises no body (no Content-Length, no Transfer-
    /// Encoding).
    content_length: Option<u64>,
    /// `Transfer-Encoding: chunked` on the request.
    is_chunked: bool,
    /// Has `Connection: upgrade` or `Upgrade:` header.
    is_upgrade: bool,
    /// Edge / client asked for explicit close on this request.
    wants_close: bool,
}

impl RequestShape {
    fn poolable(&self) -> bool {
        !self.is_chunked && !self.is_upgrade && !self.wants_close
    }
}

fn analyse_request(req: &ConnectRequest) -> RequestShape {
    let mut shape = RequestShape {
        content_length: None,
        is_chunked: false,
        is_upgrade: false,
        wants_close: false,
    };
    for (k, v) in &req.metadata {
        let Some(name) = k.strip_prefix(&format!("{HTTP_HEADER_KEY}:")) else {
            continue;
        };
        let lname = name.to_ascii_lowercase();
        let lval = v.to_ascii_lowercase();
        match lname.as_str() {
            "content-length" => {
                shape.content_length = v.parse().ok();
            }
            "transfer-encoding" if lval.contains("chunked") => {
                shape.is_chunked = true;
            }
            "upgrade" => {
                shape.is_upgrade = true;
            }
            "connection" => {
                if lval.contains("upgrade") {
                    shape.is_upgrade = true;
                }
                if lval.contains("close") {
                    shape.wants_close = true;
                }
            }
            _ => {}
        }
    }
    shape
}

#[derive(Debug, Clone)]
struct ResponseShape {
    content_length: Option<u64>,
    is_chunked: bool,
    is_upgrade: bool, // status 101 or `Connection: upgrade`
    wants_close: bool,
}

impl ResponseShape {
    fn poolable(&self) -> bool {
        self.content_length.is_some() && !self.is_chunked && !self.is_upgrade && !self.wants_close
    }
}

fn analyse_response(status: u16, headers: &[(String, String)]) -> ResponseShape {
    let mut shape = ResponseShape {
        content_length: None,
        is_chunked: false,
        is_upgrade: status == 101,
        wants_close: false,
    };
    for (name, value) in headers {
        let lname = name.to_ascii_lowercase();
        let lval = value.to_ascii_lowercase();
        match lname.as_str() {
            "content-length" => shape.content_length = value.parse().ok(),
            "transfer-encoding" if lval.contains("chunked") => {
                shape.is_chunked = true;
            }
            "connection" => {
                if lval.contains("close") {
                    shape.wants_close = true;
                }
                if lval.contains("upgrade") {
                    shape.is_upgrade = true;
                }
            }
            "upgrade" => shape.is_upgrade = true,
            _ => {}
        }
    }
    shape
}

// ── HTTP path ────────────────────────────────────────────────────────────────

async fn proxy_http<R, W>(
    router: &Router,
    request: ConnectRequest,
    from_edge: R,
    mut to_edge: W,
    counters: StreamCounters,
    pool: Arc<Pool>,
) -> Result<(), TunnelError>
where
    R: futures::io::AsyncRead + Unpin,
    W: futures::io::AsyncWrite + Unpin,
{
    let req_shape = analyse_request(&request);
    let head_request = request
        .meta(HTTP_METHOD_KEY)
        .map(|m| m.eq_ignore_ascii_case("HEAD"))
        .unwrap_or(false);

    // Resolve the origin for this request's host.
    let host = request
        .meta(HTTP_HOST_KEY)
        .map(|s| s.to_string())
        .or_else(|| host_from_dest(&request.dest))
        .unwrap_or_default();
    let target = match router.resolve(&host) {
        Some(t) => OriginTarget {
            addr: t.addr,
            tls: t.tls,
            host_header: t.host_header,
        },
        None => {
            warn!(%host, "no ingress rule matches host");
            return write_error_response(&mut to_edge, 502, &format!("no route for {host}")).await;
        }
    };

    if target.tls {
        // TLS origins are never pooled: handshake per request, always
        // bidi. Do this before acquiring from the pool.
        return proxy_http_tls(target, request, from_edge, to_edge, counters).await;
    }

    // Acquire socket (pool hit or fresh connect).
    let tcp = match tokio::time::timeout(LOCAL_CONNECT_TIMEOUT, pool.acquire(&target.addr)).await {
        Ok(Ok(s)) => s,
        Ok(Err(e)) => {
            warn!(error = %e, addr = %target.addr, "TCP connect refused");
            return write_error_response(&mut to_edge, 502, &format!("local connect: {e}")).await;
        }
        Err(_) => {
            warn!(addr = %target.addr, "TCP connect timed out");
            return write_error_response(&mut to_edge, 504, "local connect timed out").await;
        }
    };

    let (tcp_read, mut tcp_write) = tcp.into_split();

    // Synthesise + write HTTP/1.1 request head.
    let head = build_request_head(&request, req_shape.poolable(), &target, &host);
    tcp_write
        .write_all(head.as_bytes())
        .await
        .map_err(|e| TunnelError::Internal(format!("tcp write head: {e}")))?;

    if req_shape.poolable() {
        run_pooled(
            req_shape,
            head_request,
            from_edge,
            to_edge,
            tcp_read,
            tcp_write,
            counters,
            &pool,
            &target.addr,
        )
        .await
    } else {
        run_bidi(from_edge, to_edge, tcp_read, tcp_write, counters).await
    }
}

/// TLS-origin path: connect with rustls, then run the bidi pump.
/// No pooling (keep-alive across TLS is more trouble than it’s worth
/// for the local-origin case).
async fn proxy_http_tls<R, W>(
    target: OriginTarget,
    request: ConnectRequest,
    from_edge: R,
    to_edge: W,
    counters: StreamCounters,
) -> Result<(), TunnelError>
where
    R: futures::io::AsyncRead + Unpin,
    W: futures::io::AsyncWrite + Unpin,
{
    let req_shape = analyse_request(&request);
    let host = request
        .meta(HTTP_HOST_KEY)
        .map(|s| s.to_string())
        .or_else(|| host_from_dest(&request.dest))
        .unwrap_or_default();

    let tcp = match tokio::time::timeout(LOCAL_CONNECT_TIMEOUT, TcpStream::connect(&target.addr))
        .await
    {
        Ok(Ok(s)) => s,
        Ok(Err(e)) => {
            return write_error_response_t(to_edge, 502, &format!("local connect: {e}")).await
        }
        Err(_) => return write_error_response_t(to_edge, 504, "local connect timed out").await,
    };

    let server_name = target
        .addr
        .split(':')
        .next()
        .unwrap_or("localhost")
        .to_string();
    let tls = crate::origin_tls::connect(tcp, &server_name)
        .await
        .map_err(|e| TunnelError::Origin(format!("origin TLS: {e}")))?;

    let (tls_read, mut tls_write) = tokio::io::split(tls);
    let head = build_request_head(&request, req_shape.poolable(), &target, &host);
    use tokio::io::AsyncWriteExt as _;
    tls_write
        .write_all(head.as_bytes())
        .await
        .map_err(|e| TunnelError::Internal(format!("tls write head: {e}")))?;

    run_bidi_tokio(from_edge, to_edge, tls_read, tls_write, counters).await
}

#[allow(clippy::too_many_arguments)]
async fn run_pooled<R, W>(
    req_shape: RequestShape,
    head_request: bool,
    mut from_edge: R,
    mut to_edge: W,
    mut tcp_read: tokio::net::tcp::OwnedReadHalf,
    mut tcp_write: tokio::net::tcp::OwnedWriteHalf,
    counters: StreamCounters,
    pool: &Pool,
    addr: &str,
) -> Result<(), TunnelError>
where
    R: futures::io::AsyncRead + Unpin,
    W: futures::io::AsyncWrite + Unpin,
{
    let in_counter = counters.bytes_in.clone();
    let out_counter = counters.bytes_out.clone();

    // 1. Forward request body bytes. Bound by Content-Length if
    //    present; otherwise zero bytes (GET-style).
    if let Some(n) = req_shape.content_length {
        if n > 0 {
            pump_n_futures_to_tokio(&mut from_edge, &mut tcp_write, n, &in_counter).await?;
        }
    }
    // Crucially: do NOT shutdown tcp_write. We want the socket to
    // stay available for the next request from the pool.

    // 2. Read response head off TCP.
    let (status, headers, leftover) = read_http_response_head(&mut tcp_read).await?;
    debug!(status, header_count = headers.len(), "origin response");
    let resp_shape = analyse_response(status, &headers);

    // 3. Echo status + headers back to edge.
    let mut meta: Vec<(String, String)> = Vec::with_capacity(headers.len() + 1);
    meta.push((HTTP_STATUS_KEY.into(), status.to_string()));
    for (name, value) in &headers {
        meta.push((format!("{HTTP_HEADER_KEY}:{name}"), value.clone()));
    }
    let meta_refs: Vec<(&str, &str)> = meta.iter().map(|(k, v)| (k.as_str(), v.as_str())).collect();
    stream::write_connect_response(&mut to_edge, "", &meta_refs).await?;

    // 3b. HEAD has no response body by definition. The origin still
    //     advertises Content-Length (of the would-be GET body), so
    //     falling through to `pump_n_tokio_to_futures` would block
    //     waiting for bytes that never arrive and the edge would
    //     answer 502. Send the head and close immediately.
    if head_request {
        to_edge
            .close()
            .await
            .map_err(|e| TunnelError::Internal(format!("close to_edge (HEAD): {e}")))?;
        return Ok(());
    }

    // 4. Flush any header-over-read bytes (start of body) first.
    if !leftover.is_empty() {
        to_edge
            .write_all(&leftover)
            .await
            .map_err(|e| TunnelError::Internal(format!("write leftover body: {e}")))?;
        out_counter.fetch_add(leftover.len() as u64, Ordering::Relaxed);
    }

    if let Some(total) = resp_shape.content_length.filter(|_| resp_shape.poolable()) {
        // 5a. Read exactly Content-Length bytes (minus what was in
        //     `leftover`) and forward. Then release socket.
        let remaining = total.saturating_sub(leftover.len() as u64);
        if remaining > 0 {
            pump_n_tokio_to_futures(&mut tcp_read, &mut to_edge, remaining, &out_counter).await?;
        }
        to_edge
            .close()
            .await
            .map_err(|e| TunnelError::Internal(format!("close to_edge: {e}")))?;

        match tcp_read.reunite(tcp_write) {
            Ok(socket) => pool.release(socket, addr).await,
            Err(e) => {
                warn!(error = %e, "tcp halves did not reunite; dropping socket");
            }
        }
        Ok(())
    } else {
        // 5b. Response wasn't poolable after all (no Content-Length,
        //     or Upgrade, or chunked, or Connection: close). Fall
        //     through to drain-until-EOF + drop the socket.
        pump_tokio_to_futures_counted(&mut tcp_read, &mut to_edge, &out_counter)
            .await
            .ok();
        to_edge
            .close()
            .await
            .map_err(|e| TunnelError::Internal(format!("close to_edge: {e}")))?;
        Ok(())
    }
}

async fn run_bidi<R, W>(
    from_edge: R,
    to_edge: W,
    tcp_read: tokio::net::tcp::OwnedReadHalf,
    tcp_write: tokio::net::tcp::OwnedWriteHalf,
    counters: StreamCounters,
) -> Result<(), TunnelError>
where
    R: futures::io::AsyncRead + Unpin,
    W: futures::io::AsyncWrite + Unpin,
{
    run_bidi_tokio(from_edge, to_edge, tcp_read, tcp_write, counters).await
}

/// Bidi pump over any tokio-io origin halves.
async fn run_bidi_tokio<R, W, OR, OW>(
    mut from_edge: R,
    mut to_edge: W,
    mut origin_read: OR,
    mut origin_write: OW,
    counters: StreamCounters,
) -> Result<(), TunnelError>
where
    R: futures::io::AsyncRead + Unpin,
    W: futures::io::AsyncWrite + Unpin,
    OR: AsyncRead + Unpin,
    OW: AsyncWrite + Unpin,
{
    let in_counter = counters.bytes_in.clone();
    let out_counter = counters.bytes_out.clone();
    let edge_to_local = async {
        let _ = pump_futures_to_tokio_counted(&mut from_edge, &mut origin_write, &in_counter).await;
        let _ = origin_write.shutdown().await;
        Ok::<(), TunnelError>(())
    };
    let local_to_edge = async {
        let (status, headers, leftover) = read_http_response_head(&mut origin_read).await?;
        debug!(
            status,
            header_count = headers.len(),
            "origin response (bidi)"
        );
        let mut meta: Vec<(String, String)> = Vec::with_capacity(headers.len() + 1);
        meta.push((HTTP_STATUS_KEY.into(), status.to_string()));
        for (name, value) in &headers {
            meta.push((format!("{HTTP_HEADER_KEY}:{name}"), value.clone()));
        }
        let meta_refs: Vec<(&str, &str)> =
            meta.iter().map(|(k, v)| (k.as_str(), v.as_str())).collect();
        stream::write_connect_response(&mut to_edge, "", &meta_refs).await?;
        if !leftover.is_empty() {
            to_edge
                .write_all(&leftover)
                .await
                .map_err(|e| TunnelError::Internal(format!("write leftover body: {e}")))?;
            out_counter.fetch_add(leftover.len() as u64, Ordering::Relaxed);
        }
        pump_tokio_to_futures_counted(&mut origin_read, &mut to_edge, &out_counter).await
    };

    let (_, response_result) = tokio::join!(edge_to_local, local_to_edge);
    response_result?;
    to_edge
        .close()
        .await
        .map_err(|e| TunnelError::Internal(format!("close to_edge: {e}")))?;
    Ok(())
}

// ── HTTP head builder ───────────────────────────────────────────────────────

fn build_request_head(
    req: &ConnectRequest,
    keep_alive: bool,
    target: &OriginTarget,
    _host: &str,
) -> String {
    let method = req.meta(HTTP_METHOD_KEY).unwrap_or("GET");
    let host = target
        .host_header
        .clone()
        .or_else(|| req.meta(HTTP_HOST_KEY).map(|s| s.to_string()))
        .unwrap_or_default();
    let path = extract_path(&req.dest);

    let mut head = String::with_capacity(256);
    head.push_str(method);
    head.push(' ');
    head.push_str(&path);
    head.push_str(" HTTP/1.1\r\n");
    if !host.is_empty() {
        head.push_str("Host: ");
        head.push_str(&host);
        head.push_str("\r\n");
    }

    let mut saw_connection = false;
    for (k, v) in &req.metadata {
        if let Some(name) = k.strip_prefix(&format!("{HTTP_HEADER_KEY}:")) {
            if name.eq_ignore_ascii_case("host") {
                continue;
            }
            if name.eq_ignore_ascii_case("connection") {
                saw_connection = true;
            }
            head.push_str(name);
            head.push_str(": ");
            head.push_str(v);
            head.push_str("\r\n");
        }
    }
    if !saw_connection {
        if keep_alive {
            head.push_str("Connection: keep-alive\r\n");
        } else {
            head.push_str("Connection: close\r\n");
        }
    }
    head.push_str("\r\n");
    head
}

fn extract_path(dest: &str) -> String {
    if let Some(after_scheme) = dest.find("://") {
        let rest = &dest[after_scheme + 3..];
        if let Some(slash) = rest.find('/') {
            return rest[slash..].to_string();
        }
        return "/".into();
    }
    if dest.starts_with('/') {
        return dest.to_string();
    }
    "/".into()
}

fn host_from_dest(dest: &str) -> Option<String> {
    let after_scheme = dest.find("://")?;
    let rest = &dest[after_scheme + 3..];
    let authority = rest.split('/').next().unwrap_or(rest);
    if authority.is_empty() {
        None
    } else {
        Some(authority.to_string())
    }
}

async fn write_error_response<W>(writer: &mut W, status: u16, msg: &str) -> Result<(), TunnelError>
where
    W: futures::io::AsyncWrite + Unpin,
{
    let meta = [(HTTP_STATUS_KEY, status.to_string())];
    let refs: Vec<(&str, &str)> = meta.iter().map(|(k, v)| (*k, v.as_str())).collect();
    stream::write_connect_response(writer, msg, &refs).await?;
    Ok(())
}

async fn write_error_response_t<W>(mut writer: W, status: u16, msg: &str) -> Result<(), TunnelError>
where
    W: futures::io::AsyncWrite + Unpin,
{
    write_error_response(&mut writer, status, msg).await
}

async fn read_http_response_head(
    tcp: &mut (impl tokio::io::AsyncRead + Unpin),
) -> Result<(u16, Vec<(String, String)>, Vec<u8>), TunnelError> {
    let mut buf = Vec::with_capacity(4096);
    let mut tmp = [0u8; 2048];
    loop {
        let n = tcp
            .read(&mut tmp)
            .await
            .map_err(|e| TunnelError::Internal(format!("tcp read head: {e}")))?;
        if n == 0 {
            return Err(TunnelError::Internal(
                "local origin closed before sending response head".into(),
            ));
        }
        buf.extend_from_slice(&tmp[..n]);
        if buf.len() > MAX_HEADER_BYTES {
            return Err(TunnelError::Internal(format!(
                "response header exceeds {MAX_HEADER_BYTES} bytes"
            )));
        }
        let mut headers = [httparse::EMPTY_HEADER; 64];
        let mut resp = httparse::Response::new(&mut headers);
        match resp
            .parse(&buf)
            .map_err(|e| TunnelError::Internal(format!("httparse: {e}")))?
        {
            httparse::Status::Complete(consumed) => {
                let status = resp
                    .code
                    .ok_or_else(|| TunnelError::Internal("response had no status code".into()))?;
                let pairs = resp
                    .headers
                    .iter()
                    .map(|h| {
                        let v = String::from_utf8_lossy(h.value).into_owned();
                        (h.name.to_string(), v)
                    })
                    .collect::<Vec<_>>();
                let leftover = buf.split_off(consumed);
                return Ok((status, pairs, leftover));
            }
            httparse::Status::Partial => {}
        }
    }
}

// ── TCP path ────────────────────────────────────────────────────────────────

async fn proxy_tcp<R, W>(
    router: &Router,
    request: &ConnectRequest,
    from_edge: &mut R,
    to_edge: &mut W,
    counters: &StreamCounters,
) -> Result<(), TunnelError>
where
    R: futures::io::AsyncRead + Unpin,
    W: futures::io::AsyncWrite + Unpin,
{
    // For TCP the dest is `host:port`, resolved straight through the
    // router (which may pin a default for quick tunnels).
    let host = request.dest.split(':').next().unwrap_or(&request.dest);
    let target = router
        .resolve(host)
        .ok_or_else(|| TunnelError::Origin(format!("no route for tcp dest {}", request.dest)))?;
    let tcp = TcpStream::connect(&target.addr)
        .await
        .map_err(|e| TunnelError::Origin(format!("tcp connect: {e}")))?;
    let (mut r, mut w) = tcp.into_split();
    stream::write_connect_response(to_edge, "", &[]).await?;
    let edge_to_local = pump_futures_to_tokio_counted(from_edge, &mut w, &counters.bytes_in);
    let local_to_edge = pump_tokio_to_futures_counted(&mut r, to_edge, &counters.bytes_out);
    let _ = futures::future::join(edge_to_local, local_to_edge).await;
    Ok(())
}

// ── Byte pumps ──────────────────────────────────────────────────────────────

async fn pump_futures_to_tokio_counted<R, W>(
    src: &mut R,
    dst: &mut W,
    counter: &AtomicU64,
) -> Result<(), TunnelError>
where
    R: futures::io::AsyncRead + Unpin,
    W: tokio::io::AsyncWrite + Unpin,
{
    let mut buf = [0u8; 16 * 1024];
    loop {
        let n = src
            .read(&mut buf)
            .await
            .map_err(|e| TunnelError::Internal(format!("read: {e}")))?;
        if n == 0 {
            break;
        }
        dst.write_all(&buf[..n])
            .await
            .map_err(|e| TunnelError::Internal(format!("write: {e}")))?;
        counter.fetch_add(n as u64, Ordering::Relaxed);
    }
    Ok(())
}

async fn pump_tokio_to_futures_counted<R, W>(
    src: &mut R,
    dst: &mut W,
    counter: &AtomicU64,
) -> Result<(), TunnelError>
where
    R: tokio::io::AsyncRead + Unpin,
    W: futures::io::AsyncWrite + Unpin,
{
    let mut buf = [0u8; 16 * 1024];
    loop {
        let n = src
            .read(&mut buf)
            .await
            .map_err(|e| TunnelError::Internal(format!("read: {e}")))?;
        if n == 0 {
            break;
        }
        dst.write_all(&buf[..n])
            .await
            .map_err(|e| TunnelError::Internal(format!("write: {e}")))?;
        counter.fetch_add(n as u64, Ordering::Relaxed);
    }
    Ok(())
}

/// Forward exactly `n` bytes from futures-io source to tokio-io
/// dest. Fails if the source EOFs early.
async fn pump_n_futures_to_tokio<R, W>(
    src: &mut R,
    dst: &mut W,
    mut n: u64,
    counter: &AtomicU64,
) -> Result<(), TunnelError>
where
    R: futures::io::AsyncRead + Unpin,
    W: tokio::io::AsyncWrite + Unpin,
{
    let mut buf = [0u8; 16 * 1024];
    while n > 0 {
        let want = std::cmp::min(buf.len() as u64, n) as usize;
        let read = src
            .read(&mut buf[..want])
            .await
            .map_err(|e| TunnelError::Internal(format!("read: {e}")))?;
        if read == 0 {
            return Err(TunnelError::Internal(format!(
                "source EOF with {n} bytes still expected"
            )));
        }
        dst.write_all(&buf[..read])
            .await
            .map_err(|e| TunnelError::Internal(format!("write: {e}")))?;
        counter.fetch_add(read as u64, Ordering::Relaxed);
        n -= read as u64;
    }
    Ok(())
}

/// Forward exactly `n` bytes from tokio-io source to futures-io
/// dest. Same shape as the request-side helper.
async fn pump_n_tokio_to_futures<R, W>(
    src: &mut R,
    dst: &mut W,
    mut n: u64,
    counter: &AtomicU64,
) -> Result<(), TunnelError>
where
    R: tokio::io::AsyncRead + Unpin,
    W: futures::io::AsyncWrite + Unpin,
{
    let mut buf = [0u8; 16 * 1024];
    while n > 0 {
        let want = std::cmp::min(buf.len() as u64, n) as usize;
        let read = src
            .read(&mut buf[..want])
            .await
            .map_err(|e| TunnelError::Internal(format!("read: {e}")))?;
        if read == 0 {
            return Err(TunnelError::Internal(format!(
                "tcp EOF with {n} bytes still expected"
            )));
        }
        dst.write_all(&buf[..read])
            .await
            .map_err(|e| TunnelError::Internal(format!("write: {e}")))?;
        counter.fetch_add(read as u64, Ordering::Relaxed);
        n -= read as u64;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::router::parse_service;

    #[test]
    fn extract_path_strips_scheme() {
        assert_eq!(
            extract_path("https://abc.trycloudflare.com/path?q=1"),
            "/path?q=1"
        );
        assert_eq!(extract_path("https://abc.trycloudflare.com"), "/");
        assert_eq!(extract_path("/relative/x"), "/relative/x");
    }

    #[test]
    fn host_from_dest_works() {
        assert_eq!(
            host_from_dest("https://app.example.com:443/x?y=1").unwrap(),
            "app.example.com:443"
        );
        assert_eq!(host_from_dest("abc.trycloudflare.com").is_none(), true);
    }

    #[test]
    fn build_head_includes_method_host_path() {
        let req = ConnectRequest {
            dest: "https://abc.trycloudflare.com/foo".into(),
            conn_type: ConnectionType::Http,
            metadata: vec![
                (HTTP_METHOD_KEY.into(), "POST".into()),
                (HTTP_HOST_KEY.into(), "abc.trycloudflare.com".into()),
                (format!("{HTTP_HEADER_KEY}:User-Agent"), "x/1".into()),
                (format!("{HTTP_HEADER_KEY}:X-Stuff"), "yo".into()),
            ],
        };
        let target = OriginTarget {
            addr: "127.0.0.1:8080".into(),
            tls: false,
            host_header: None,
        };
        let head = build_request_head(&req, true, &target, "abc.trycloudflare.com");
        assert!(head.starts_with("POST /foo HTTP/1.1\r\n"));
        assert!(head.contains("Host: abc.trycloudflare.com\r\n"));
        assert!(head.contains("User-Agent: x/1\r\n"));
        assert!(head.contains("X-Stuff: yo\r\n"));
        assert!(head.contains("Connection: keep-alive\r\n"));
        assert!(head.ends_with("\r\n\r\n"));
    }

    #[test]
    fn poolable_request_default() {
        let req = ConnectRequest {
            dest: "https://x/".into(),
            conn_type: ConnectionType::Http,
            metadata: vec![
                (HTTP_METHOD_KEY.into(), "GET".into()),
                (HTTP_HOST_KEY.into(), "x".into()),
            ],
        };
        let s = analyse_request(&req);
        assert!(s.poolable());
        assert_eq!(s.content_length, None);
    }

    #[test]
    fn websocket_request_not_poolable() {
        let req = ConnectRequest {
            dest: "https://x/ws".into(),
            conn_type: ConnectionType::Websocket,
            metadata: vec![
                (HTTP_METHOD_KEY.into(), "GET".into()),
                (HTTP_HOST_KEY.into(), "x".into()),
                (format!("{HTTP_HEADER_KEY}:Upgrade"), "websocket".into()),
                (format!("{HTTP_HEADER_KEY}:Connection"), "Upgrade".into()),
            ],
        };
        let s = analyse_request(&req);
        assert!(s.is_upgrade);
        assert!(!s.poolable());
    }

    #[test]
    fn chunked_request_not_poolable() {
        let req = ConnectRequest {
            dest: "https://x/upload".into(),
            conn_type: ConnectionType::Http,
            metadata: vec![
                (HTTP_METHOD_KEY.into(), "POST".into()),
                (
                    format!("{HTTP_HEADER_KEY}:Transfer-Encoding"),
                    "chunked".into(),
                ),
            ],
        };
        let s = analyse_request(&req);
        assert!(s.is_chunked);
        assert!(!s.poolable());
    }

    #[test]
    fn response_with_content_length_is_poolable() {
        let hs = vec![("Content-Length".into(), "42".into())];
        let s = analyse_response(200, &hs);
        assert!(s.poolable());
        assert_eq!(s.content_length, Some(42));
    }

    #[test]
    fn response_101_never_poolable() {
        let hs = vec![("Upgrade".into(), "websocket".into())];
        let s = analyse_response(101, &hs);
        assert!(s.is_upgrade);
        assert!(!s.poolable());
    }

    #[test]
    fn router_default_and_mapping() {
        let r = Router::fixed(9000);
        assert_eq!(r.resolve("anything").unwrap().addr, "127.0.0.1:9000");
        let mut m = std::collections::HashMap::new();
        m.insert("a.example.com".into(), parse_service("http://127.0.0.1:1").unwrap());
        let r2 = Router::from_map(m, None);
        assert!(r2.resolve("a.example.com").is_some());
        assert!(r2.resolve("b.example.com").is_none());
    }
}

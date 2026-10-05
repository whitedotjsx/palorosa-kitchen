//! Minimal HTTPS client for the one POST we actually make.
//!
//! `reqwest` (hyper + h2 + tower) dominated the binary for a single
//! request path. This is a hand-rolled HTTP/1.1 client over
//! `tokio` + `rustls` (the same rustls the crate already uses for
//! QUIC and HTTPS origins), with a fast-path TLS setup: system trust
//! store is loaded lazily and cached for the process.
//!
//! Scope is deliberately tiny: one request, `Content-Type:
//! application/json`, `Connection: close`, full-body read via
//! `Content-Length` or chunked transfer. No redirects, no cookie
//! jar, no HTTP/2, no proxies. That is exactly what
//! `POST api.trycloudflare.com/tunnel` needs.

use std::sync::Arc;
use std::time::Duration;

use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};
use tokio::net::TcpStream;
use tokio_rustls::rustls::pki_types::{CertificateDer, ServerName};
use tokio_rustls::rustls::{ClientConfig, RootCertStore};
use tokio_rustls::TlsConnector;

/// Transport-level failure for the API call. Mapped 1:1 into
/// [`crate::error::TunnelError::Api`].
#[derive(Debug, thiserror::Error)]
pub enum HttpError {
    #[error("invalid url: {0}")]
    Url(String),
    #[error("dns failure: {0}")]
    Dns(String),
    #[error("connect failure: {0}")]
    Connect(String),
    #[error("tls failure: {0}")]
    Tls(String),
    #[error("io failure: {0}")]
    Io(String),
    #[error("request timed out")]
    Timeout,
    #[error("malformed response: {0}")]
    BadResponse(String),
}

impl HttpError {
    /// Transient errors are worth a retry (network / DNS / TLS /
    /// timeout). Malformed responses and URL mistakes are not.
    pub fn is_transient(&self) -> bool {
        matches!(
            self,
            HttpError::Dns(_)
                | HttpError::Connect(_)
                | HttpError::Tls(_)
                | HttpError::Io(_)
                | HttpError::Timeout
        )
    }
}

/// Perform a single request. `headers` are additional request
/// headers (e.g. `User-Agent`, `Content-Type`). Returns
/// `(status_code, body_bytes)`.
pub async fn request(
    url: &str,
    method: &str,
    headers: &[(&str, &str)],
    timeout: Duration,
) -> Result<(u16, Vec<u8>), HttpError> {
    tokio::time::timeout(timeout, request_inner(url, method, headers))
        .await
        .map_err(|_| HttpError::Timeout)?
}

async fn request_inner(
    url: &str,
    method: &str,
    headers: &[(&str, &str)],
) -> Result<(u16, Vec<u8>), HttpError> {
    let (scheme, host, port, path) = parse_url(url)?;

    let tcp = TcpStream::connect((host.as_str(), port))
        .await
        .map_err(|e| HttpError::Connect(format!("{host}:{port}: {e}")))?;
    let _ = tcp.set_nodelay(true);

    let req = build_request(method, &scheme, &host, port, &path, headers);

    if scheme == "https" {
        match tls_connect(tcp, &host).await {
            Ok(stream) => do_io(stream, req.as_bytes()).await,
            Err(FastTlsError::Failed(e)) => Err(e),
            Err(FastTlsError::Tcp(tcp)) => do_io(tcp, req.as_bytes()).await,
        }
    } else {
        do_io(tcp, req.as_bytes()).await
    }
}

/// Outcome of a TLS attempt that may fall back to plain TCP (used
/// only for `http://` sanity paths during tests).
enum FastTlsError {
    Failed(HttpError),
    Tcp(TcpStream),
}

async fn tls_connect(tcp: TcpStream, host: &str) -> Result<tokio_rustls::client::TlsStream<TcpStream>, FastTlsError> {
    let cfg = tls_client_config().map_err(FastTlsError::Failed)?;
    let connector = TlsConnector::from(cfg);
    let sn = ServerName::try_from(host.to_string())
        .map_err(|e| FastTlsError::Failed(HttpError::Tls(format!("server name {host}: {e}"))))?;
    connector
        .connect(sn, tcp)
        .await
        .map_err(|e| FastTlsError::Failed(HttpError::Tls(format!("handshake: {e}"))))
}

fn tls_client_config() -> Result<Arc<ClientConfig>, HttpError> {
    // rustls 0.23 needs a process-level CryptoProvider selected.
    // reqwest used to pull it in transitively; now we install ring
    // explicitly (the same provider QUIC uses).
    static PROVIDER: std::sync::Once = std::sync::Once::new();
    PROVIDER.call_once(|| {
        let _ = tokio_rustls::rustls::crypto::ring::default_provider().install_default();
    });

    // Build once; the trust store does not change within a process.
    static CFG: std::sync::OnceLock<Arc<ClientConfig>> = std::sync::OnceLock::new();
    if let Some(cfg) = CFG.get() {
        return Ok(cfg.clone());
    }
    let mut roots = RootCertStore::empty();
    match rustls_native_certs::load_native_certs() {
        Ok(certs) => {
            for c in certs {
                let _ = roots.add(CertificateDer::from(c.to_vec()));
            }
        }
        Err(e) => return Err(HttpError::Tls(format!("native certs: {e}"))),
    }
    if roots.is_empty() {
        return Err(HttpError::Tls("no trust anchors available".into()));
    }
    let cfg = Arc::new(
        ClientConfig::builder()
            .with_root_certificates(roots)
            .with_no_client_auth(),
    );
    let _ = CFG.set(cfg.clone());
    Ok(cfg)
}

fn build_request(
    method: &str,
    scheme: &str,
    host: &str,
    port: u16,
    path: &str,
    headers: &[(&str, &str)],
) -> String {
    let default_port = (scheme == "https" && port == 443) || (scheme == "http" && port == 80);
    let host_header = if default_port {
        host.to_string()
    } else {
        format!("{host}:{port}")
    };

    let mut req = String::with_capacity(256);
    req.push_str(method);
    req.push(' ');
    req.push_str(path);
    req.push_str(" HTTP/1.1\r\nHost: ");
    req.push_str(&host_header);
    req.push_str("\r\nConnection: close\r\n");
    for (k, v) in headers {
        req.push_str(k);
        req.push_str(": ");
        req.push_str(v);
        req.push_str("\r\n");
    }
    req.push_str("\r\n");
    req
}

async fn do_io<S>(mut s: S, req: &[u8]) -> Result<(u16, Vec<u8>), HttpError>
where
    S: AsyncRead + AsyncWrite + Unpin,
{
    s.write_all(req).await.map_err(|e| HttpError::Io(e.to_string()))?;
    s.flush().await.map_err(|e| HttpError::Io(e.to_string()))?;
    read_response(&mut s).await
}

async fn read_response<S: AsyncRead + Unpin>(s: &mut S) -> Result<(u16, Vec<u8>), HttpError> {
    let mut buf: Vec<u8> = Vec::with_capacity(8192);
    let mut tmp = [0u8; 8192];

    // 1. Accumulate until end-of-headers.
    let head_end = loop {
        if let Some(pos) = find_subsequence(&buf, b"\r\n\r\n") {
            break pos;
        }
        let n = s.read(&mut tmp).await.map_err(|e| HttpError::Io(e.to_string()))?;
        if n == 0 {
            return Err(HttpError::BadResponse("connection closed before headers".into()));
        }
        buf.extend_from_slice(&tmp[..n]);
        if buf.len() > 128 * 1024 {
            return Err(HttpError::BadResponse("response headers too large".into()));
        }
    };

    let head = buf[..head_end + 2].to_vec(); // include final CRLF of head
    let mut body: Vec<u8> = buf[head_end + 4..].to_vec();

    let mut hdrs = [httparse::EMPTY_HEADER; 64];
    let mut resp = httparse::Response::new(&mut hdrs);
    resp.parse(&head)
        .map_err(|e| HttpError::BadResponse(format!("httparse: {e}")))?;
    let status = resp
        .code
        .ok_or_else(|| HttpError::BadResponse("missing status code".into()))?;

    let mut content_length: Option<usize> = None;
    let mut chunked = false;
    for h in resp.headers.iter() {
        if h.name.eq_ignore_ascii_case("content-length") {
            content_length = std::str::from_utf8(h.value)
                .ok()
                .and_then(|v| v.trim().parse::<usize>().ok());
        } else if h.name.eq_ignore_ascii_case("transfer-encoding") {
            let v = std::str::from_utf8(h.value).unwrap_or("");
            if v.to_ascii_lowercase().contains("chunked") {
                chunked = true;
            }
        }
    }

    // 2. Read the body.
    if chunked {
        loop {
            match try_decode_chunked(&body) {
                Ok(Some(decoded)) => return Ok((status, decoded)),
                Ok(None) => {
                    let n = s.read(&mut tmp).await.map_err(|e| HttpError::Io(e.to_string()))?;
                    if n == 0 {
                        return Ok((status, body));
                    }
                    body.extend_from_slice(&tmp[..n]);
                }
                Err(e) => return Err(e),
            }
        }
    }

    let want = content_length.unwrap_or(usize::MAX);
    while body.len() < want {
        let n = s.read(&mut tmp).await.map_err(|e| HttpError::Io(e.to_string()))?;
        if n == 0 {
            break;
        }
        body.extend_from_slice(&tmp[..n]);
    }
    if body.len() > want {
        body.truncate(want);
    }
    Ok((status, body))
}

fn try_decode_chunked(data: &[u8]) -> Result<Option<Vec<u8>>, HttpError> {
    let mut out = Vec::new();
    let mut i = 0usize;
    loop {
        let line_end = match find_subsequence(&data[i..], b"\r\n") {
            Some(p) => i + p,
            None => return Ok(None),
        };
        let size_line = &data[i..line_end];
        let size_str = size_line.split(|b| *b == b';').next().unwrap_or(b"");
        let size_str = std::str::from_utf8(size_str)
            .map_err(|_| HttpError::BadResponse("chunk size not utf8".into()))?
            .trim();
        let size = usize::from_str_radix(size_str, 16)
            .map_err(|_| HttpError::BadResponse(format!("bad chunk size: {size_str:?}")))?;
        i = line_end + 2;
        if size == 0 {
            return Ok(Some(out));
        }
        if i + size + 2 > data.len() {
            return Ok(None);
        }
        out.extend_from_slice(&data[i..i + size]);
        i += size + 2;
    }
}

fn parse_url(url: &str) -> Result<(String, String, u16, String), HttpError> {
    let (scheme, rest) = url
        .split_once("://")
        .ok_or_else(|| HttpError::Url(format!("missing scheme: {url}")))?;
    let scheme = scheme.to_ascii_lowercase();
    if scheme != "http" && scheme != "https" {
        return Err(HttpError::Url(format!("unsupported scheme: {scheme}")));
    }

    let (authority, path) = match rest.find('/') {
        Some(i) => (&rest[..i], &rest[i..]),
        None => (rest, "/"),
    };

    // Drop any userinfo (not used here, but keep the parse honest).
    let authority = authority.rsplit('@').next().unwrap_or(authority);

    let (host, port) = match authority.rsplit_once(':') {
        Some((h, p)) if !h.is_empty() => (
            h.to_string(),
            p.parse::<u16>()
                .map_err(|_| HttpError::Url(format!("bad port: {p}")))?,
        ),
        _ => (
            authority.to_string(),
            if scheme == "https" { 443 } else { 80 },
        ),
    };

    if host.is_empty() {
        return Err(HttpError::Url("empty host".into()));
    }
    Ok((scheme, host, port, path.to_string()))
}

fn find_subsequence(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    if needle.is_empty() || haystack.len() < needle.len() {
        return None;
    }
    haystack
        .windows(needle.len())
        .position(|w| w == needle)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_http_and_https() {
        assert_eq!(
            parse_url("https://api.trycloudflare.com/tunnel").unwrap(),
            ("https".into(), "api.trycloudflare.com".into(), 443, "/tunnel".into())
        );
        assert_eq!(
            parse_url("http://127.0.0.1:8080/tunnel").unwrap(),
            ("http".into(), "127.0.0.1".into(), 8080, "/tunnel".into())
        );
        assert_eq!(
            parse_url("https://example.com").unwrap(),
            ("https".into(), "example.com".into(), 443, "/".into())
        );
        assert!(parse_url("ftp://x").is_err());
        assert!(parse_url("no-scheme").is_err());
    }

    #[test]
    fn decodes_simple_chunked_body() {
        let data = b"4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n";
        assert_eq!(
            try_decode_chunked(data).unwrap(),
            Some(b"Wikipedia".to_vec())
        );
    }

    #[test]
    fn chunked_returns_none_when_incomplete() {
        let data = b"4\r\nWi";
        assert_eq!(try_decode_chunked(data).unwrap(), None);
    }

    #[test]
    fn builds_host_header_with_port() {
        let r = build_request("POST", "http", "127.0.0.1", 8888, "/tunnel", &[]);
        assert!(r.contains("Host: 127.0.0.1:8888\r\n"));
        let r = build_request("POST", "https", "api.example.com", 443, "/tunnel", &[]);
        assert!(r.contains("Host: api.example.com\r\n"));
    }
}

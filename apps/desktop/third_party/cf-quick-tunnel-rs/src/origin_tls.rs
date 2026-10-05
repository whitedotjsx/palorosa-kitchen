//! TLS client for `https://` origins.
//!
//! Named-tunnel ingress rules may point at an HTTPS origin
//! (`service: https://origin.internal:443`). cloudflared speaks TLS
//! to such origins (optionally verifying against `originRequest` CA
//! settings); we do the same using the system trust store.
//!
//! The rustls crypto provider is installed once process-wide by
//! `quic_dial` (ring); this module reuses it.

use std::sync::Arc;

use tokio::net::TcpStream;
use tokio_rustls::rustls::pki_types::{CertificateDer, ServerName};
use tokio_rustls::rustls::{ClientConfig, RootCertStore};
use tokio_rustls::TlsConnector;

/// Establish a TLS session to `tcp`, validating against the system
/// roots and using `server_name` for SNI + verification.
pub async fn connect(
    tcp: TcpStream,
    server_name: &str,
) -> Result<tokio_rustls::client::TlsStream<TcpStream>, String> {
    let mut roots = RootCertStore::empty();
    match rustls_native_certs::load_native_certs() {
        Ok(certs) => {
            for c in certs {
                let _ = roots.add(CertificateDer::from(c.to_vec()));
            }
        }
        Err(e) => tracing::warn!(error = %e, "origin TLS: native certs unavailable"),
    }

    let cfg = ClientConfig::builder()
        .with_root_certificates(roots)
        .with_no_client_auth();
    let connector = TlsConnector::from(Arc::new(cfg));

    let sn = ServerName::try_from(server_name.to_string())
        .map_err(|e| format!("invalid server name {server_name}: {e}"))?;
    connector
        .connect(sn, tcp)
        .await
        .map_err(|e| format!("handshake: {e}"))
}

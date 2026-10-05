//! Top-level orchestrators.
//!
//! - [`QuickTunnelManager`] — anonymous `*.trycloudflare.com` tunnels
//!   (credentials fetched from the trycloudflare API, one fixed
//!   origin).
//! - [`NamedTunnelManager`] — named tunnels bound to a custom
//!   hostname (credentials from a token / credentials file; ingress
//!   either local or pushed by the edge).
//!
//! Both funnel into [`start_tunnel`], which runs:
//!
//!   1. edge discovery
//!   2. QUIC dial + register (one per HA conn-index)
//!   3. spawn a reactor per connection: supervise, reconnect on drop
//!
//! The reactor is the long-lived owner: it cycles between
//! "supervise current connection" and "reconnect with backoff +
//! `replace_existing=true`" until shutdown or the reconnect budget
//! runs out.

use std::sync::Arc;
use std::time::Duration;

use quinn::Endpoint;
use tokio::sync::oneshot;
use tracing::{debug, info, warn};
use uuid::Uuid;

use crate::api::{request_tunnel, DEFAULT_SERVICE_URL, DEFAULT_USER_AGENT};
use crate::credentials::TunnelCredentials;
use crate::edge::{discover, IpVersionFilter};
use crate::error::TunnelError;
use crate::ingress;
use crate::pool::Pool;
use crate::quic_dial::{build_endpoint, dial_any};
use crate::router::Router;
use crate::rpc::{register_connection, ConnectionOptions, ControlSession, TunnelAuth};
use crate::rpc_config::{self, SharedRouter};
use crate::supervisor::{self, SupervisorExit, SupervisorMetrics};

/// Default budget for POST + discovery + handshake + register.
pub const DEFAULT_HANDSHAKE_TIMEOUT: Duration = Duration::from_secs(30);

/// Default budget for the `unregisterConnection` RPC on shutdown.
pub const DEFAULT_GRACE_PERIOD: Duration = Duration::from_secs(30);

/// Hard cap on consecutive reconnect failures before the reactor
/// gives up. Each failure widens the backoff (1s → 30s, exponential
/// with a cap).
pub const MAX_RECONNECT_ATTEMPTS: u32 = 10;

/// How many parallel QUIC connections to register, each on a
/// distinct `conn_index`. Higher = more resilient to a single edge
/// POP dropping. cloudflared uses 4 for named tunnels and 1 for
/// quick tunnels; we default to 4 for named and 2 for quick.
pub const DEFAULT_HA_CONNECTIONS: u8 = 2;
pub const DEFAULT_HA_CONNECTIONS_NAMED: u8 = 4;

/// Hard ceiling — matches cloudflared.
pub const MAX_HA_CONNECTIONS: u8 = 4;

/// Crate version, baked into `ConnectionOptions.client.version`.
pub const CLIENT_VERSION: &str = concat!("cloudflare-quick-tunnel/", env!("CARGO_PKG_VERSION"));

#[derive(Debug, Clone, Default)]
pub struct TunnelMetrics {
    pub streams_total: u64,
    pub bytes_in: u64,
    pub bytes_out: u64,
    pub reconnects: u64,
}

/// A live tunnel. Returned by both managers.
pub struct TunnelHandle {
    /// Public URL. For quick tunnels this is the generated
    /// `*.trycloudflare.com`; for named tunnels the first configured
    /// hostname (or empty when the edge hasn't pushed config yet).
    pub url: Option<String>,
    pub tunnel_id: Uuid,
    pub account_tag: String,
    /// Edge POP location of the first registered connection.
    pub location: String,
    shutdown: Arc<tokio::sync::Notify>,
    reactors: Vec<tokio::task::JoinHandle<()>>,
    metrics_view: SupervisorMetrics,
    reconnects: Arc<std::sync::atomic::AtomicU64>,
    /// Shared router, kept so callers can read/override it.
    router: SharedRouter,
}

impl TunnelHandle {
    pub fn metrics(&self) -> TunnelMetrics {
        let (s, i, o) = self.metrics_view.snapshot();
        TunnelMetrics {
            streams_total: s,
            bytes_in: i,
            bytes_out: o,
            reconnects: self.reconnects.load(std::sync::atomic::Ordering::Relaxed),
        }
    }

    pub fn router(&self) -> SharedRouter {
        self.router.clone()
    }

    pub async fn shutdown_with(mut self, _grace: Duration) -> Result<(), TunnelError> {
        self.shutdown.notify_waiters();
        for j in self.reactors.drain(..) {
            j.await
                .map_err(|e| TunnelError::Internal(format!("reactor join: {e}")))?;
        }
        Ok(())
    }

    pub async fn shutdown(self) -> Result<(), TunnelError> {
        self.shutdown_with(DEFAULT_GRACE_PERIOD).await
    }
}

impl Drop for TunnelHandle {
    fn drop(&mut self) {
        self.shutdown.notify_waiters();
    }
}

/// Back-compat alias.
pub type QuickTunnelHandle = TunnelHandle;

/// Back-compat alias for the quick manager's result.
pub type NamedTunnelHandle = TunnelHandle;

// ── Quick tunnel manager ─────────────────────────────────────────────────────

pub struct QuickTunnelManager {
    pub local_port: u16,
    pub discovery_timeout: Duration,
    pub service_url: String,
    pub user_agent: String,
    pub ha_connections: u8,
}

impl QuickTunnelManager {
    pub fn new(local_port: u16) -> Self {
        Self {
            local_port,
            discovery_timeout: DEFAULT_HANDSHAKE_TIMEOUT,
            service_url: DEFAULT_SERVICE_URL.into(),
            user_agent: DEFAULT_USER_AGENT.into(),
            ha_connections: DEFAULT_HA_CONNECTIONS,
        }
    }

    pub fn with_timeout(mut self, d: Duration) -> Self {
        self.discovery_timeout = d;
        self
    }

    pub fn with_service_url(mut self, url: impl Into<String>) -> Self {
        self.service_url = url.into();
        self
    }

    pub fn with_user_agent(mut self, ua: impl Into<String>) -> Self {
        self.user_agent = ua.into();
        self
    }

    pub fn with_ha_connections(mut self, n: u8) -> Self {
        self.ha_connections = n.clamp(1, MAX_HA_CONNECTIONS);
        self
    }

    pub async fn start(self) -> Result<TunnelHandle, TunnelError> {
        tokio::time::timeout(self.discovery_timeout, self.start_inner())
            .await
            .map_err(|_| TunnelError::Internal("start() exceeded discovery_timeout".into()))?
    }

    async fn start_inner(self) -> Result<TunnelHandle, TunnelError> {
        let tunnel = request_tunnel(&self.service_url, &self.user_agent).await?;
        info!(hostname = %tunnel.hostname, id = %tunnel.id, ha = self.ha_connections, "got quick tunnel");
        let tunnel_id = Uuid::parse_str(&tunnel.id)
            .map_err(|e| TunnelError::Internal(format!("tunnel.id is not a uuid: {e}")))?;
        let url = if tunnel.hostname.starts_with("https://") {
            tunnel.hostname.clone()
        } else {
            format!("https://{}", tunnel.hostname)
        };

        let auth = TunnelAuth {
            account_tag: tunnel.account_tag.clone(),
            tunnel_secret: tunnel.secret.clone(),
        };
        let router = rpc_config::shared_router(Router::fixed(self.local_port));

        let handle = start_tunnel(StartParams {
            auth,
            tunnel_id,
            account_tag: tunnel.account_tag,
            url: Some(url),
            router,
            ha_connections: self.ha_connections,
            discovery_timeout: self.discovery_timeout,
        })
        .await?;
        Ok(handle)
    }
}

// ── Named tunnel manager ─────────────────────────────────────────────────────

/// Configuration for a named tunnel.
pub struct NamedTunnelConfig {
    pub credentials: TunnelCredentials,
    /// Static ingress (YAML/JSON). Empty for a remotely-managed
    /// tunnel whose config the edge will push.
    pub ingress: String,
    pub ha_connections: u8,
    pub discovery_timeout: Duration,
}

impl NamedTunnelConfig {
    pub fn new(credentials: TunnelCredentials) -> Self {
        Self {
            credentials,
            ingress: String::new(),
            ha_connections: DEFAULT_HA_CONNECTIONS_NAMED,
            discovery_timeout: DEFAULT_HANDSHAKE_TIMEOUT,
        }
    }

    /// Load a token or credentials file from a string.
    pub fn from_token_or_file(input: &str) -> Result<Self, TunnelError> {
        Ok(Self::new(crate::credentials::parse_auto(input)?))
    }

    pub fn with_ingress(mut self, ingress: impl Into<String>) -> Self {
        self.ingress = ingress.into();
        self
    }

    pub fn with_ha_connections(mut self, n: u8) -> Self {
        self.ha_connections = n.clamp(1, MAX_HA_CONNECTIONS);
        self
    }

    pub fn with_timeout(mut self, d: Duration) -> Self {
        self.discovery_timeout = d;
        self
    }
}

pub struct NamedTunnelManager {
    config: NamedTunnelConfig,
}

impl NamedTunnelManager {
    pub fn new(config: NamedTunnelConfig) -> Self {
        Self { config }
    }

    pub async fn start(self) -> Result<TunnelHandle, TunnelError> {
        let cfg = self.config;
        tokio::time::timeout(cfg.discovery_timeout, async move {
            let router = rpc_config::shared_router(if cfg.ingress.trim().is_empty() {
                Router::default()
            } else {
                ingress::build_router(&cfg.ingress)?
            });

            let auth = TunnelAuth {
                account_tag: cfg.credentials.account_tag.clone(),
                tunnel_secret: cfg.credentials.tunnel_secret.clone(),
            };

            start_tunnel(StartParams {
                auth,
                tunnel_id: cfg.credentials.tunnel_id,
                account_tag: cfg.credentials.account_tag,
                url: first_hostname(&cfg.ingress),
                router,
                ha_connections: cfg.ha_connections,
                discovery_timeout: cfg.discovery_timeout,
            })
            .await
        })
        .await
        .map_err(|_| TunnelError::Internal("named start() exceeded discovery_timeout".into()))?
    }
}

/// Pull the first `hostname:` from an ingress doc for a display URL.
fn first_hostname(ingress: &str) -> Option<String> {
    for line in ingress.lines() {
        let Some(idx) = line.find("hostname") else {
            continue;
        };
        // Find the ':' that follows the key, then read the value.
        let Some(colon) = line[idx..].find(':') else {
            continue;
        };
        let v = line[idx + colon + 1..]
            .trim()
            .trim_end_matches(',')
            .trim_matches('"')
            .trim_matches('\'')
            .trim();
        // In JSON the value ends at the closing quote; slice there.
        let v = v.split('"').next().unwrap_or(v).trim().trim_matches('\'');
        if !v.is_empty() {
            return Some(format!("https://{v}"));
        }
    }
    None
}

// ── Shared core ──────────────────────────────────────────────────────────────

struct StartParams {
    auth: TunnelAuth,
    tunnel_id: Uuid,
    account_tag: String,
    url: Option<String>,
    router: SharedRouter,
    ha_connections: u8,
    discovery_timeout: Duration,
}

async fn start_tunnel(p: StartParams) -> Result<TunnelHandle, TunnelError> {
    let endpoint = build_endpoint()?;

    let (conn0, control0, location0) =
        connect_cycle(&endpoint, &p.auth, p.tunnel_id, CLIENT_VERSION, 0, false, p.router.clone())
            .await?;
    info!(%location0, conn_index = 0, "first registration succeeded");

    let metrics = SupervisorMetrics::default();
    let reconnects = Arc::new(std::sync::atomic::AtomicU64::new(0));
    let shutdown = Arc::new(tokio::sync::Notify::new());
    let pool = Arc::new(Pool::new());

    let mut reactors = Vec::with_capacity(p.ha_connections as usize);
    reactors.push(tokio::spawn(reactor_loop(
        endpoint.clone(),
        p.auth.clone(),
        p.tunnel_id,
        0,
        metrics.clone(),
        reconnects.clone(),
        pool.clone(),
        p.router.clone(),
        conn0,
        control0,
        shutdown.clone(),
    )));

    for idx in 1..p.ha_connections {
        let endpoint = endpoint.clone();
        let auth = p.auth.clone();
        let metrics = metrics.clone();
        let reconnects = reconnects.clone();
        let shutdown = shutdown.clone();
        let pool = pool.clone();
        let router = p.router.clone();
        let tunnel_id = p.tunnel_id;
        reactors.push(tokio::spawn(async move {
            match connect_cycle(&endpoint, &auth, tunnel_id, CLIENT_VERSION, idx, false, router.clone())
                .await
            {
                Ok((conn, control, location)) => {
                    info!(%location, conn_index = idx, "HA registration succeeded");
                    reactor_loop(
                        endpoint, auth, tunnel_id, idx, metrics, reconnects, pool, router, conn,
                        control, shutdown,
                    )
                    .await;
                }
                Err(e) => {
                    warn!(error = %e, conn_index = idx, "HA registration failed; will retry");
                    reactor_loop_after_failure(
                        endpoint, auth, tunnel_id, idx, metrics, reconnects, pool, router, shutdown,
                    )
                    .await;
                }
            }
        }));
    }

    Ok(TunnelHandle {
        url: p.url,
        tunnel_id: p.tunnel_id,
        account_tag: p.account_tag,
        location: location0,
        shutdown,
        reactors,
        metrics_view: metrics,
        reconnects,
        router: p.router,
    })
}

#[allow(clippy::too_many_arguments)]
async fn connect_cycle(
    endpoint: &Endpoint,
    auth: &TunnelAuth,
    tunnel_id: Uuid,
    client_version: &str,
    conn_index: u8,
    replace_existing: bool,
    router: SharedRouter,
) -> Result<(quinn::Connection, ControlSession, String), TunnelError> {
    let edges = discover(IpVersionFilter::Auto).await?;
    let cap = edges.len().min(5);
    let conn = dial_any(endpoint, &edges[..cap]).await?;

    let mut options = ConnectionOptions::default_for_quick_tunnel(client_version);
    options.replace_existing = replace_existing;

    let (details, control) =
        register_connection(&conn, auth, tunnel_id, conn_index, &options, router).await?;
    Ok((conn, control, details.location))
}

#[allow(clippy::too_many_arguments)]
async fn reactor_loop(
    endpoint: Endpoint,
    auth: TunnelAuth,
    tunnel_id: Uuid,
    conn_index: u8,
    metrics: SupervisorMetrics,
    reconnects: Arc<std::sync::atomic::AtomicU64>,
    pool: Arc<Pool>,
    router: SharedRouter,
    mut conn: quinn::Connection,
    mut control: ControlSession,
    shutdown: Arc<tokio::sync::Notify>,
) {
    debug!(conn_index, "reactor loop started");
    loop {
        let (sup_tx, sup_rx) = oneshot::channel();
        let metrics_for_cycle = metrics.clone();
        let shutdown_wait = shutdown.notified();
        tokio::pin!(shutdown_wait);
        let exit = tokio::select! {
            biased;
            _ = &mut shutdown_wait => {
                let _ = sup_tx.send(());
                SupervisorExit::Shutdown
            }
            exit = supervisor::run(conn, router.clone(), metrics_for_cycle, pool.clone(), sup_rx) => exit,
        };

        match exit {
            SupervisorExit::Shutdown => {
                control.shutdown_graceful(DEFAULT_GRACE_PERIOD).await;
                debug!(conn_index, "reactor: clean shutdown");
                return;
            }
            SupervisorExit::ConnectionLost => {
                drop(control);

                let mut attempt = 0u32;
                loop {
                    attempt += 1;
                    if attempt > MAX_RECONNECT_ATTEMPTS {
                        warn!(
                            conn_index,
                            "reactor: giving up after {} reconnect attempts",
                            MAX_RECONNECT_ATTEMPTS
                        );
                        return;
                    }
                    let delay = backoff(attempt);
                    warn!(conn_index, attempt, ?delay, "reactor: scheduling reconnect");
                    let shutdown_wait = shutdown.notified();
                    tokio::pin!(shutdown_wait);
                    tokio::select! {
                        biased;
                        _ = shutdown_wait => {
                            debug!(conn_index, "reactor: shutdown during reconnect backoff");
                            return;
                        }
                        _ = tokio::time::sleep(delay) => {}
                    }
                    match connect_cycle(
                        &endpoint,
                        &auth,
                        tunnel_id,
                        CLIENT_VERSION,
                        conn_index,
                        true,
                        router.clone(),
                    )
                    .await
                    {
                        Ok((new_conn, new_control, new_loc)) => {
                            info!(conn_index, attempt, location = %new_loc, "reactor: reconnect succeeded");
                            reconnects.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
                            conn = new_conn;
                            control = new_control;
                            break;
                        }
                        Err(e) => {
                            warn!(attempt, error = %e, "reactor: reconnect failed");
                        }
                    }
                }
            }
        }
    }
}

#[allow(clippy::too_many_arguments)]
async fn reactor_loop_after_failure(
    endpoint: Endpoint,
    auth: TunnelAuth,
    tunnel_id: Uuid,
    conn_index: u8,
    metrics: SupervisorMetrics,
    reconnects: Arc<std::sync::atomic::AtomicU64>,
    pool: Arc<Pool>,
    router: SharedRouter,
    shutdown: Arc<tokio::sync::Notify>,
) {
    let mut attempt = 0u32;
    loop {
        attempt += 1;
        if attempt > MAX_RECONNECT_ATTEMPTS {
            warn!(conn_index, "HA reactor: giving up after {} initial-register attempts", MAX_RECONNECT_ATTEMPTS);
            return;
        }
        let delay = backoff(attempt);
        warn!(conn_index, attempt, ?delay, "HA reactor: scheduling initial register retry");
        let shutdown_wait = shutdown.notified();
        tokio::pin!(shutdown_wait);
        tokio::select! {
            biased;
            _ = shutdown_wait => return,
            _ = tokio::time::sleep(delay) => {}
        }
        let result =
            connect_cycle(&endpoint, &auth, tunnel_id, CLIENT_VERSION, conn_index, false, router.clone())
                .await;
        match result {
            Ok((conn, control, location)) => {
                info!(conn_index, %location, "HA leg eventually registered after {attempt} retries");
                reactor_loop(
                    endpoint, auth, tunnel_id, conn_index, metrics, reconnects, pool, router, conn,
                    control, shutdown.clone(),
                )
                .await;
                return;
            }
            Err(e) => warn!(conn_index, attempt, error = %e, "HA register retry failed"),
        }
    }
}

/// Exponential backoff with a 30s ceiling: 1s, 2s, 4s, 8s, 16s, 30s, …
fn backoff(attempt: u32) -> Duration {
    let secs = 1u64.checked_shl(attempt.saturating_sub(1)).unwrap_or(30);
    Duration::from_secs(secs.min(30))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn backoff_curve() {
        assert_eq!(backoff(1), Duration::from_secs(1));
        assert_eq!(backoff(2), Duration::from_secs(2));
        assert_eq!(backoff(3), Duration::from_secs(4));
        assert_eq!(backoff(4), Duration::from_secs(8));
        assert_eq!(backoff(5), Duration::from_secs(16));
        assert_eq!(backoff(6), Duration::from_secs(30));
        assert_eq!(backoff(20), Duration::from_secs(30));
    }

    #[test]
    fn first_hostname_yaml_and_json() {
        assert_eq!(
            first_hostname("ingress:\n  - hostname: a.example.com\n"),
            Some("https://a.example.com".into())
        );
        assert_eq!(
            first_hostname(r#"{"ingress":[{"hostname":"b.example.com"}]}"#),
            Some("https://b.example.com".into())
        );
        assert_eq!(first_hostname("ingress: []"), None);
    }
}

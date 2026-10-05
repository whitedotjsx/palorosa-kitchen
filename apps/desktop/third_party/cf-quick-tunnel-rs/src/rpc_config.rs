//! Implements the `ConfigurationManager` RPC the edge calls on named
//! tunnels to push configuration.
//!
//! For a **remotely-managed** named tunnel the ingress rules live in
//! Cloudflare, not on disk. On connect (and on later edits) the edge
//! invokes `updateConfiguration(version, config)` with the JSON blob;
//! we parse its `ingress` list and hot-swap the shared [`Router`] so
//! subsequent inbound streams route to the new origins.
//!
//! The quick-tunnel path never sees this call (the edge only pushes
//! config to named tunnels). We still serve the interface so the
//! edge’s liveness/bootstrap resolution succeeds.

use std::sync::{Arc, RwLock};

use capnp::capability::Promise;
use tracing::{info, warn};

use crate::ingress;
use crate::router::Router;
use crate::tunnelrpc_capnp;

/// Shared, hot-swappable routing table handed to the proxy and the
/// supervisor. A `std::sync::RwLock` is deliberate: reads happen on
/// the hot path (per request) and never hold across an await, and the
/// `ConfigurationManager` handler swaps it synchronously.
pub type SharedRouter = Arc<RwLock<Arc<Router>>>;

pub fn shared_router(initial: Router) -> SharedRouter {
    Arc::new(RwLock::new(Arc::new(initial)))
}

/// Read the current router (cheap Arc clone).
pub fn current(shared: &SharedRouter) -> Arc<Router> {
    shared.read().expect("router rwlock poisoned").clone()
}

/// Swap the router atomically.
pub fn apply(shared: &SharedRouter, new: Router) {
    *shared.write().expect("router rwlock poisoned") = Arc::new(new);
}

/// Server impl for `CloudflaredServer` (which extends
/// `ConfigurationManager` + `SessionManager`). Only
/// `updateConfiguration` is meaningful for HTTP tunnels;
/// session methods stay unimplemented.
pub struct ConfigServer {
    router: SharedRouter,
}

impl ConfigServer {
    pub fn new(router: SharedRouter) -> Self {
        Self { router }
    }
}

impl tunnelrpc_capnp::configuration_manager::Server for ConfigServer {
    fn update_configuration(
        &mut self,
        params: tunnelrpc_capnp::configuration_manager::UpdateConfigurationParams,
        mut results: tunnelrpc_capnp::configuration_manager::UpdateConfigurationResults,
    ) -> Promise<(), capnp::Error> {
        let reader = match params.get() {
            Ok(r) => r,
            Err(e) => return ack_error(results, format!("bad updateConfiguration params: {e}")),
        };
        let version = reader.get_version();
        let config_bytes = match reader.get_config() {
            Ok(b) => b.to_vec(),
            Err(e) => return ack_error(results, format!("no config field: {e}")),
        };
        let config_str = String::from_utf8_lossy(&config_bytes).into_owned();

        info!(version, bytes = config_bytes.len(), "edge pushed configuration");

        match ingress::build_router(&config_str) {
            Ok(router) => {
                apply(&self.router, router);
                let mut res = results.get();
                let mut out = res.reborrow().init_result();
                out.set_latest_applied_version(version);
                out.set_err("");
                Promise::ok(())
            }
            Err(e) => {
                warn!(error = %e, "pushed configuration failed to parse; keeping previous");
                ack_error(results, format!("{e}"))
            }
        }
    }
}

impl tunnelrpc_capnp::session_manager::Server for ConfigServer {}
impl tunnelrpc_capnp::cloudflared_server::Server for ConfigServer {}

fn ack_error(
    mut results: tunnelrpc_capnp::configuration_manager::UpdateConfigurationResults,
    err: String,
) -> Promise<(), capnp::Error> {
    let mut res = results.get();
    let mut out = res.reborrow().init_result();
    out.set_latest_applied_version(-1);
    out.set_err(&err);
    Promise::ok(())
}

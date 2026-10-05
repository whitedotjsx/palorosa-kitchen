//! Origin routing.
//!
//! Quick tunnels have a single, fixed origin (`127.0.0.1:<port>`) so
//! every inbound stream goes to the same place. Named tunnels route
//! by **hostname**: the edge tells us the destination URL/host on the
//! `ConnectRequest`, and we map it to a local service derived from
//! the tunnel’s ingress rules.
//!
//! [`Router`] captures both modes. The proxy calls
//! [`Router::resolve`] with the request host and gets back a target
//! string it can connect to.

use std::collections::HashMap;
use std::sync::Arc;

/// A resolved origin service.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Service {
    /// `host:port` to TCP-connect (e.g. `127.0.0.1:8081`).
    pub addr: String,
    /// `true` when the ingress rule used `https://` and we therefore
    /// must speak TLS to the origin.
    pub tls: bool,
    /// Host header to send to the origin (from `httpHostHeader` if
    /// present, else the tunnel hostname).
    pub host_header: Option<String>,
}

#[derive(Clone, Default)]
pub struct Router {
    /// lowercased hostname -> service
    map: Arc<HashMap<String, Service>>,
    /// Fallback for hosts with no explicit rule (quick tunnels and
    /// named tunnels with a `catch-all` / single rule).
    default: Option<Service>,
}

impl Router {
    /// Quick-tunnel mode: everything to a fixed local port.
    pub fn fixed(port: u16) -> Self {
        Self {
            map: Arc::new(HashMap::new()),
            default: Some(Service {
                addr: format!("127.0.0.1:{port}"),
                tls: false,
                host_header: None,
            }),
        }
    }

    /// Named-tunnel mode from an explicit hostname map, plus an
    /// optional default service.
    pub fn from_map(map: HashMap<String, Service>, default: Option<Service>) -> Self {
        let map = map
            .into_iter()
            .map(|(k, v)| (k.to_ascii_lowercase(), v))
            .collect();
        Self {
            map: Arc::new(map),
            default,
        }
    }

    /// Resolve a request host (may include `:port`) to a service.
    pub fn resolve(&self, host: &str) -> Option<Service> {
        let bare = host.split(':').next().unwrap_or(host).to_ascii_lowercase();
        self.map
            .get(&bare)
            .cloned()
            .or_else(|| self.default.clone())
    }

    pub fn is_empty(&self) -> bool {
        self.map.is_empty() && self.default.is_none()
    }
}

/// Parse a cloudflared ingress `service` string into a [`Service`].
///
/// Supported forms (the common ones for HTTP tunnels):
///   - `http://127.0.0.1:8080`      → plain TCP, no TLS
///   - `https://origin.internal:443`→ TLS to the origin
///   - `http://localhost:8080`
///   - `127.0.0.1:8080`             → plain TCP
pub fn parse_service(service: &str) -> Option<Service> {
    let s = service.trim();
    let (tls, rest) = if let Some(r) = s.strip_prefix("https://") {
        (true, r)
    } else if let Some(r) = s.strip_prefix("http://") {
        (false, r)
    } else {
        (false, s)
    };
    // Strip any path (ingress services are authority-only in practice).
    let authority = rest.split('/').next().unwrap_or(rest);
    // Must be `host:port` with a numeric port; reject pseudo-services
    // (`http_status:404`), bare schemes (`unix:`, `tcp:`), and hosts
    // that can't be DNS names.
    let (host, port) = authority.rsplit_once(':')?;
    if host.is_empty() || host.contains('_') {
        return None;
    }
    if port.parse::<u16>().is_err() {
        return None;
    }
    Some(Service {
        addr: authority.to_string(),
        tls,
        host_header: None,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fixed_router_resolves_anything() {
        let r = Router::fixed(8080);
        let s = r.resolve("whatever.example.com").unwrap();
        assert_eq!(s.addr, "127.0.0.1:8080");
        assert!(!s.tls);
    }

    #[test]
    fn named_router_maps_and_defaults() {
        let mut m = HashMap::new();
        m.insert("app.example.com".into(), parse_service("http://127.0.0.1:8081").unwrap());
        let r = Router::from_map(m, Some(parse_service("http://127.0.0.1:9090").unwrap()));
        assert_eq!(r.resolve("app.example.com").unwrap().addr, "127.0.0.1:8081");
        assert_eq!(r.resolve("app.example.com:443").unwrap().addr, "127.0.0.1:8081");
        assert_eq!(r.resolve("other.example.com").unwrap().addr, "127.0.0.1:9090");
    }

    #[test]
    fn parses_service_forms() {
        assert_eq!(parse_service("http://localhost:8080").unwrap().addr, "localhost:8080");
        assert!(parse_service("https://x:443").unwrap().tls);
        assert_eq!(parse_service("127.0.0.1:8080").unwrap().addr, "127.0.0.1:8080");
        assert!(parse_service("unix:/tmp/sock").is_none());
    }
}

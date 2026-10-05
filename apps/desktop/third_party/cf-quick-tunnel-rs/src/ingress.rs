//! Ingress configuration parsing for named tunnels.
//!
//! cloudflared named tunnels carry an `ingress` list: an ordered set
//! of `{hostname?, service, originRequest?}` rules ending in a
//! catch-all (`service: http_status:404`, or a bare service). We
//! translate the HTTP-relevant subset into a [`Router`].
//!
//! Two sources feed this module:
//!
//!   1. A **local** config file (`config.yml`) — YAML.
//!   2. A **remote** config pushed by the edge via the
//!      `ConfigurationManager.updateConfiguration` RPC — JSON.
//!
//! JSON is a subset of YAML, so `serde_yaml` parses both. This means
//! one code path handles locally- and remotely-managed tunnels.

use std::collections::HashMap;

use serde::Deserialize;

use crate::error::TunnelError;
use crate::router::{parse_service, Router, Service};

#[derive(Debug, Deserialize)]
struct IngressDoc {
    #[serde(default)]
    ingress: Vec<Rule>,
}

#[derive(Debug, Deserialize)]
struct Rule {
    #[serde(default)]
    hostname: Option<String>,
    service: String,
    #[serde(default, rename = "originRequest")]
    origin_request: Option<OriginRequest>,
}

#[derive(Debug, Deserialize)]
struct OriginRequest {
    #[serde(default, rename = "httpHostHeader")]
    http_host_header: Option<String>,
    // Other originRequest knobs (noTLSVerify, connectTimeout, …) are
    // accepted but not yet acted on; we only need the Host override
    // for correct routing.
}

/// Parse an ingress document (YAML or JSON) into a [`Router`].
///
/// The first rule *with* a hostname becomes a mapping; a rule
/// *without* a hostname is the catch-all / default. Non-HTTP
/// services (e.g. `http_status:404`) are skipped from mapping but a
/// bare non-HTTP catch-all is ignored so the router simply returns
/// `None` for unmatched hosts (→ our proxy answers 502).
pub fn build_router(doc: &str) -> Result<Router, TunnelError> {
    // Empty document / no ingress → empty router.
    let trimmed = doc.trim();
    if trimmed.is_empty() {
        return Ok(Router::default());
    }

    let parsed: IngressDoc = serde_yaml::from_str(trimmed)
        .map_err(|e| TunnelError::Config(format!("ingress parse: {e}")))?;

    let mut map: HashMap<String, Service> = HashMap::new();
    let mut default: Option<Service> = None;

    for rule in &parsed.ingress {
        // `http_status:NNN` and other non-origin pseudo-services have
        // no `host:port`; parse_service returns None for schemes we
        // don't proxy.
        let mut svc = match parse_service(&rule.service) {
            Some(s) => s,
            None => continue,
        };
        if let Some(or) = &rule.origin_request {
            if let Some(hh) = &or.http_host_header {
                svc.host_header = Some(hh.clone());
            }
        }
        match &rule.hostname {
            Some(h) if !h.is_empty() => {
                map.insert(h.to_ascii_lowercase(), svc);
            }
            _ => {
                // Catch-all rule (last rule usually). First one wins.
                if default.is_none() {
                    default = Some(svc);
                }
            }
        }
    }

    Ok(Router::from_map(map, default))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_json_ingress() {
        let json = r#"{
            "ingress": [
                {"hostname": "app.example.com", "service": "http://127.0.0.1:8081",
                 "originRequest": {"httpHostHeader": "internal.local"}},
                {"service": "http://127.0.0.1:9999"}
            ]
        }"#;
        let r = build_router(json).unwrap();
        let a = r.resolve("app.example.com").unwrap();
        assert_eq!(a.addr, "127.0.0.1:8081");
        assert_eq!(a.host_header.as_deref(), Some("internal.local"));
        assert_eq!(r.resolve("other.example.com").unwrap().addr, "127.0.0.1:9999");
    }

    #[test]
    fn parses_yaml_ingress() {
        let yaml = "
ingress:
  - hostname: a.example.com
    service: http://127.0.0.1:3000
  - service: http_status:404
";
        let r = build_router(yaml).unwrap();
        assert_eq!(r.resolve("a.example.com").unwrap().addr, "127.0.0.1:3000");
        // http_status catch-all is not a routable origin.
        assert!(r.resolve("z.example.com").is_none());
    }

    #[test]
    fn empty_doc_is_empty_router() {
        assert!(build_router("").unwrap().is_empty());
    }
}

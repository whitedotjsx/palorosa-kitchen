//! Tunnel credentials for **named** tunnels.
//!
//! A named tunnel is identified by three values the edge expects in
//! the `TunnelAuth` blob on `registerConnection`:
//!
//!   - `account_tag`   — the Cloudflare account id
//!   - `tunnel_id`     — the tunnel's UUID
//!   - `tunnel_secret` — 32 raw bytes (delivered base64 in both
//!                       the token and the credentials file)
//!
//! Two on-disk / on-wire shapes are accepted, mirroring cloudflared:
//!
//!   1. **Tunnel token** (what the dashboard gives you): a base64
//!      blob whose decoded bytes are JSON
//!      `{"a": "<account>", "t": "<tunnel-uuid>", "s": "<b64 secret>"}`.
//!   2. **Credentials file** (`<tunnel-id>.json`, written by
//!      `cloudflared tunnel create`):
//!      `{"AccountTag": "...", "TunnelID": "...", "TunnelSecret": "<b64>"}`.

use base64::engine::general_purpose::{STANDARD, URL_SAFE, URL_SAFE_NO_PAD};
use base64::Engine;
use tracing::debug;
use uuid::Uuid;

use crate::error::TunnelError;

/// Resolved credentials ready to hand to `TunnelAuth`.
#[derive(Debug, Clone)]
pub struct TunnelCredentials {
    pub account_tag: String,
    pub tunnel_id: Uuid,
    /// Raw 32-byte tunnel secret (already base64-decoded).
    pub tunnel_secret: Vec<u8>,
}

impl TunnelCredentials {
    pub fn from_parts(
        account_tag: impl Into<String>,
        tunnel_id: &str,
        secret_b64: &str,
    ) -> Result<Self, TunnelError> {
        let tunnel_id = Uuid::parse_str(tunnel_id.trim())
            .map_err(|e| TunnelError::Credentials(format!("tunnel id is not a uuid: {e}")))?;
        let tunnel_secret = decode_b64(secret_b64.trim())
            .map_err(|e| TunnelError::Credentials(format!("tunnel secret b64: {e}")))?;
        Ok(Self {
            account_tag: account_tag.into(),
            tunnel_id,
            tunnel_secret,
        })
    }
}

/// Parse a tunnel token (the string you paste into the dashboard’s
/// `TUNNEL_TOKEN` / `cloudflared tunnel run --token`).
pub fn parse_token(token: &str) -> Result<TunnelCredentials, TunnelError> {
    let token = token.trim();
    if token.is_empty() {
        return Err(TunnelError::Credentials("empty tunnel token".into()));
    }
    // The dashboard token is base64 (standard, padded). Accept the
    // url-safe alphabet too in case a caller re-encoded it.
    let raw = decode_b64(token)
        .map_err(|e| TunnelError::Credentials(format!("token is not base64: {e}")))?;

    #[derive(serde::Deserialize)]
    struct TokenJson {
        a: String,
        t: String,
        s: String,
    }
    let parsed: TokenJson = serde_json::from_slice(&raw)
        .map_err(|e| TunnelError::Credentials(format!("token JSON {{a,t,s}}: {e}")))?;
    let creds = TunnelCredentials::from_parts(parsed.a, &parsed.t, &parsed.s)?;
    debug!(account = %creds.account_tag, tunnel_id = %creds.tunnel_id, "parsed tunnel token");
    Ok(creds)
}

/// Parse a `cloudflared tunnel create` credentials file (JSON with
/// `AccountTag` / `TunnelID` / `TunnelSecret`).
pub fn parse_credentials_file(json: &str) -> Result<TunnelCredentials, TunnelError> {
    #[derive(serde::Deserialize)]
    struct CredsJson {
        #[serde(rename = "AccountTag")]
        account_tag: String,
        #[serde(rename = "TunnelID")]
        tunnel_id: String,
        #[serde(rename = "TunnelSecret")]
        tunnel_secret: String,
    }
    let parsed: CredsJson = serde_json::from_str(json)
        .map_err(|e| TunnelError::Credentials(format!("credentials JSON: {e}")))?;
    let creds =
        TunnelCredentials::from_parts(parsed.account_tag, &parsed.tunnel_id, &parsed.tunnel_secret)?;
    debug!(account = %creds.account_tag, tunnel_id = %creds.tunnel_id, "parsed credentials file");
    Ok(creds)
}

/// Try the credentials file first (it’s JSON), then the token.
/// Handy when the caller doesn’t know which shape they hold.
pub fn parse_auto(input: &str) -> Result<TunnelCredentials, TunnelError> {
    let t = input.trim();
    if t.starts_with('{') {
        parse_credentials_file(t)
    } else {
        parse_token(t)
    }
}

fn decode_b64(s: &str) -> Result<Vec<u8>, base64::DecodeError> {
    STANDARD
        .decode(s)
        .or_else(|_| URL_SAFE.decode(s))
        .or_else(|_| URL_SAFE_NO_PAD.decode(s))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_dashboard_token() {
        // {"a":"acct","t":"<uuid>","s":"<b64 32 bytes>"}
        let secret = STANDARD.encode([7u8; 32]);
        let json = format!(
            r#"{{"a":"acct-1","t":"8f6d3c2a-1111-4d2e-9b9b-aaaaaaaaaaaa","s":"{secret}"}}"#
        );
        let token = STANDARD.encode(json.as_bytes());
        let c = parse_token(&token).unwrap();
        assert_eq!(c.account_tag, "acct-1");
        assert_eq!(c.tunnel_secret, vec![7u8; 32]);
        assert_eq!(c.tunnel_id.to_string(), "8f6d3c2a-1111-4d2e-9b9b-aaaaaaaaaaaa");
    }

    #[test]
    fn parses_credentials_file() {
        let secret = STANDARD.encode([9u8; 32]);
        let json = format!(
            r#"{{"AccountTag":"acct-2","TunnelID":"8f6d3c2a-1111-4d2e-9b9b-bbbbbbbbbbbb","TunnelSecret":"{secret}"}}"#
        );
        let c = parse_credentials_file(&json).unwrap();
        assert_eq!(c.account_tag, "acct-2");
        assert_eq!(c.tunnel_secret, vec![9u8; 32]);
    }

    #[test]
    fn auto_detects() {
        // Credentials-file shape (starts with '{').
        let secret = STANDARD.encode([1u8; 32]);
        let creds = format!(
            r#"{{"AccountTag":"a","TunnelID":"8f6d3c2a-1111-4d2e-9b9b-cccccccccccc","TunnelSecret":"{secret}"}}"#
        );
        assert!(parse_auto(&creds).is_ok());

        // Token shape: base64 of JSON {a,t,s} (does NOT start with '{').
        let token_json = format!(
            r#"{{"a":"a","t":"8f6d3c2a-1111-4d2e-9b9b-cccccccccccc","s":"{secret}"}}"#
        );
        let token = STANDARD.encode(token_json.as_bytes());
        assert!(parse_auto(&token).is_ok());
    }

    #[test]
    fn rejects_garbage() {
        assert!(parse_token("not base64 !!!").is_err());
    }
}

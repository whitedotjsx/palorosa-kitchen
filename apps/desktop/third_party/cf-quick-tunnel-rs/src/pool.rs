//! Bounded idle-TCP-connection pool against local origins.
//!
//! Reduces socket() + connect() overhead per inbound stream by
//! reusing keep-alive connections to the local origin. Each pooled
//! entry carries its target address and a release timestamp; entries
//! older than `idle_ttl` are reaped on acquire so we never hand back
//! a connection the origin has already half-closed by idle timeout.
//!
//! Since named tunnels can route to several origins, the pool is
//! **address-keyed**: `acquire(addr)` only ever returns a socket that
//! was previously opened to that same `addr`.
//!
//! The pool only stores connections that are known to be in a
//! healthy "ready for next request" state — the proxy must call
//! [`Pool::release`] **only** after fully reading a response whose
//! framing it understood (Content-Length-bounded).

use std::time::{Duration, Instant};

use tokio::net::TcpStream;
use tokio::sync::Mutex;
use tracing::trace;

/// Idle socket TTL — most servers (e.g. axum, hyper, nginx) idle
/// out keep-alive connections at 60-75s; 30s gives us a safe
/// buffer and matches the typical client-side default.
pub const DEFAULT_IDLE_TTL: Duration = Duration::from_secs(30);

/// Soft cap on idle sockets per pool. Beyond this we drop the
/// freshly-released socket on the floor; the next acquire will
/// open a new one if needed.
pub const DEFAULT_MAX_IDLE: usize = 32;

struct Idle {
    stream: TcpStream,
    addr: String,
    released_at: Instant,
}

pub struct Pool {
    idle_ttl: Duration,
    max_idle: usize,
    idle: Mutex<Vec<Idle>>,
}

impl Pool {
    pub fn new() -> Self {
        Self {
            idle_ttl: DEFAULT_IDLE_TTL,
            max_idle: DEFAULT_MAX_IDLE,
            idle: Mutex::new(Vec::new()),
        }
    }

    /// Acquire a socket to `addr`: pop a fresh idle entry for that
    /// same address if available, otherwise open a new TCP connection.
    ///
    /// Every candidate is liveness-probed before being handed back:
    /// many local origins (e.g. Python's `http.server`, which speaks
    /// HTTP/1.0 and closes after each response) half-close the socket
    /// the instant they've written the response. Reusing such a socket
    /// produces an intermittent 502 (the next request on a dead
    /// connection races the FIN). A short non-blocking `peek` detects
    /// the peer's FIN and we drop the entry instead of reusing it.
    pub async fn acquire(&self, addr: &str) -> std::io::Result<TcpStream> {
        loop {
            let candidate = {
                let mut g = self.idle.lock().await;
                let mut picked = None;
                let mut i = 0;
                while i < g.len() {
                    let stale = g[i].released_at.elapsed() > self.idle_ttl;
                    let wrong = g[i].addr != addr;
                    if stale {
                        trace!(addr, "pool: evicting stale-by-TTL entry");
                        g.swap_remove(i);
                        continue;
                    }
                    if wrong {
                        i += 1;
                        continue;
                    }
                    // Candidate for this addr (most-recent first: scan
                    // from the end like a LIFO stack).
                    let entry = g.swap_remove(i);
                    picked = Some(entry.stream);
                    break;
                }
                picked
            };
            match candidate {
                Some(stream) => {
                    if is_socket_alive(&stream).await {
                        trace!(addr, "pool hit (alive)");
                        return Ok(stream);
                    }
                    trace!(addr, "pool hit but origin closed; discarding");
                }
                None => {
                    trace!(addr, "pool miss; opening fresh TCP");
                    return TcpStream::connect(addr).await;
                }
            }
        }
    }

    /// Return a socket to the pool. Drop on overflow.
    pub async fn release(&self, stream: TcpStream, addr: &str) {
        let mut g = self.idle.lock().await;
        if g.len() >= self.max_idle {
            trace!(addr, "pool full; dropping released stream");
            return;
        }
        g.push(Idle {
            stream,
            addr: addr.to_string(),
            released_at: Instant::now(),
        });
        trace!(addr, pool_size = g.len(), "pool released");
    }
}

impl Default for Pool {
    fn default() -> Self {
        Self::new()
    }
}

/// A pooled socket is reusable only if the origin has not half-closed
/// its side. Probe with a short non-blocking `peek`:
///
/// - `Ok(0)`  → the peer sent FIN; the socket is dead, drop it.
/// - `Ok(n>0)`→ unsolicited bytes are pending; don't trust the framing, drop it.
/// - `Err`    → socket error, drop it.
/// - timeout  → idle with no pending data, which is exactly the
///   healthy keep-alive state; reuse it.
async fn is_socket_alive(stream: &TcpStream) -> bool {
    let mut buf = [0u8; 1];
    match tokio::time::timeout(Duration::from_millis(5), stream.peek(&mut buf)).await {
        Ok(Ok(0)) => false,
        Ok(Ok(_)) => false,
        Ok(Err(_)) => false,
        Err(_) => true,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::AsyncWriteExt;
    use tokio::net::TcpListener;

    /// Pool hits should reuse the same socket for the same addr.
    #[tokio::test]
    async fn acquire_after_release_returns_same_stream() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let port = listener.local_addr().unwrap().port();
        tokio::spawn(async move {
            loop {
                // Accept and hold open, sending nothing: models a
                // quiet keep-alive origin (no pending bytes).
                match listener.accept().await {
                    Ok((s, _)) => std::mem::forget(s),
                    Err(_) => break,
                }
            }
        });

        let addr = format!("127.0.0.1:{port}");
        let pool = Pool::new();
        let s1 = pool.acquire(&addr).await.unwrap();
        let s1_local = s1.local_addr().unwrap();
        pool.release(s1, &addr).await;

        let s2 = pool.acquire(&addr).await.unwrap();
        assert_eq!(s2.local_addr().unwrap(), s1_local, "should reuse socket");
    }

    #[tokio::test]
    async fn does_not_cross_addresses() {
        let l1 = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let l2 = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let a1 = format!("127.0.0.1:{}", l1.local_addr().unwrap().port());
        let a2 = format!("127.0.0.1:{}", l2.local_addr().unwrap().port());
        tokio::spawn(async move { loop { let _ = l1.accept().await; } });
        tokio::spawn(async move { loop { let _ = l2.accept().await; } });

        let pool = Pool::new();
        let s1 = pool.acquire(&a1).await.unwrap();
        let s1_local = s1.local_addr().unwrap();
        pool.release(s1, &a1).await;

        // Acquiring for a2 must open a fresh socket, not reuse the a1 one.
        let s2 = pool.acquire(&a2).await.unwrap();
        assert_ne!(s2.local_addr().unwrap(), s1_local);
    }

    #[tokio::test]
    async fn pool_evicts_stale_entries() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = format!("127.0.0.1:{}", listener.local_addr().unwrap().port());
        tokio::spawn(async move {
            loop {
                let _ = listener.accept().await;
            }
        });
        let pool = Pool::new();
        // Force short TTL via a private-ish path: rebuild with custom value.
        let pool = Pool {
            idle_ttl: Duration::from_millis(50),
            ..pool
        };

        let s1 = pool.acquire(&addr).await.unwrap();
        let s1_local = s1.local_addr().unwrap();
        pool.release(s1, &addr).await;

        tokio::time::sleep(Duration::from_millis(100)).await;
        let s2 = pool.acquire(&addr).await.unwrap();
        assert_ne!(
            s2.local_addr().unwrap(),
            s1_local,
            "stale entry should have been evicted"
        );
    }
}

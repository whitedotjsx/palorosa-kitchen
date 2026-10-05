//! C ABI used by the Palorosa desktop app (Go) to run a named Cloudflare
//! tunnel in process, with no `cloudflared` subprocess.
//!
//! The Go side extracts the built cdylib and calls these functions with
//! `syscall`. Every function is thread safe: the opaque handle owns a mutex.

use std::ffi::{c_char, CStr};
use std::sync::atomic::{AtomicI32, Ordering};
use std::sync::{Arc, Mutex};

use tokio::runtime::Runtime;
use tokio::sync::oneshot;

use crate::manager::{NamedTunnelConfig, NamedTunnelManager};

/// Tunnel stopped or never started.
pub const CFT_STOPPED: i32 = 0;
/// The QUIC/RPC handshake is in progress.
pub const CFT_STARTING: i32 = 1;
/// At least one edge connection is registered.
pub const CFT_RUNNING: i32 = 2;
/// Start failed; see `cft_error_text`.
pub const CFT_ERRORED: i32 = 3;
/// Shutdown was requested; the unregister RPC is in flight.
pub const CFT_STOPPING: i32 = 4;

struct Tunnel {
    // The runtime must outlive the task that drives the tunnel.
    _runtime: Runtime,
    state: Arc<AtomicI32>,
    error: Arc<Mutex<String>>,
    url: Arc<Mutex<String>>,
    stop: Option<oneshot::Sender<()>>,
}

/// Opaque handle owned by the Go binding.
pub struct CftTunnel {
    inner: Mutex<Option<Tunnel>>,
}

/// Creates a handle. Release it with `cft_free`.
#[no_mangle]
pub extern "C" fn cft_new() -> *mut CftTunnel {
    Box::into_raw(Box::new(CftTunnel {
        inner: Mutex::new(None),
    }))
}

/// Stops any running tunnel and releases the handle.
#[no_mangle]
pub unsafe extern "C" fn cft_free(handle: *mut CftTunnel) {
    if handle.is_null() {
        return;
    }
    let _ = cft_stop(handle);
    drop(Box::from_raw(handle));
}

/// Starts a named tunnel. `credential` is a dashboard token or the JSON of a
/// `cloudflared tunnel create` credentials file; `ingress` is the YAML/JSON
/// ingress list. Returns the new state (normally `CFT_STARTING`).
#[no_mangle]
pub unsafe extern "C" fn cft_start(
    handle: *mut CftTunnel,
    credential: *const c_char,
    ingress: *const c_char,
) -> i32 {
    if handle.is_null() {
        return CFT_ERRORED;
    }
    let tunnel = &*handle;
    let credential = match read_cstr(credential) {
        Ok(value) => value,
        Err(err) => {
            set_error(tunnel, &err);
            return CFT_ERRORED;
        }
    };
    let ingress = read_cstr(ingress).unwrap_or_default();

    let runtime = match Runtime::new() {
        Ok(runtime) => runtime,
        Err(err) => {
            set_error(tunnel, &format!("tokio runtime: {err}"));
            return CFT_ERRORED;
        }
    };

    let state = Arc::new(AtomicI32::new(CFT_STARTING));
    let error = Arc::new(Mutex::new(String::new()));
    let url = Arc::new(Mutex::new(String::new()));
    let (stop, stop_rx) = oneshot::channel::<()>();

    let task_state = Arc::clone(&state);
    let task_error = Arc::clone(&error);
    let task_url = Arc::clone(&url);
    runtime.spawn(async move {
        let config = match NamedTunnelConfig::from_token_or_file(&credential) {
            Ok(config) => config.with_ingress(ingress),
            Err(err) => {
                *lock(&task_error) = err.to_string();
                task_state.store(CFT_ERRORED, Ordering::SeqCst);
                return;
            }
        };
        match NamedTunnelManager::new(config).start().await {
            Ok(active) => {
                if let Some(public) = active.url.clone() {
                    *lock(&task_url) = public;
                }
                task_state.store(CFT_RUNNING, Ordering::SeqCst);
                let _ = stop_rx.await;
                task_state.store(CFT_STOPPING, Ordering::SeqCst);
                let _ = active.shutdown().await;
                task_state.store(CFT_STOPPED, Ordering::SeqCst);
            }
            Err(err) => {
                *lock(&task_error) = err.to_string();
                task_state.store(CFT_ERRORED, Ordering::SeqCst);
            }
        }
    });

    let mut inner = lock(&tunnel.inner);
    inner.replace(Tunnel {
        _runtime: runtime,
        state,
        error,
        url,
        stop: Some(stop),
    });
    CFT_STARTING
}

/// Requests a graceful shutdown. Returns the resulting state (`CFT_STOPPING`).
#[no_mangle]
pub unsafe extern "C" fn cft_stop(handle: *mut CftTunnel) -> i32 {
    if handle.is_null() {
        return CFT_STOPPED;
    }
    let tunnel = &*handle;
    let mut inner = lock(&tunnel.inner);
    if let Some(active) = inner.as_mut() {
        if let Some(stop) = active.stop.take() {
            let _ = stop.send(());
        }
        active.state.store(CFT_STOPPING, Ordering::SeqCst);
    }
    CFT_STOPPING
}

/// Current state, one of the `CFT_*` constants.
#[no_mangle]
pub unsafe extern "C" fn cft_state(handle: *mut CftTunnel) -> i32 {
    if handle.is_null() {
        return CFT_STOPPED;
    }
    let tunnel = &*handle;
    let inner = lock(&tunnel.inner);
    match inner.as_ref() {
        Some(active) => active.state.load(Ordering::SeqCst),
        None => CFT_STOPPED,
    }
}

/// Copies the last error into `buffer` (NUL terminated) and returns its
/// length. Returns 0 when there is no error.
#[no_mangle]
pub unsafe extern "C" fn cft_error_text(handle: *mut CftTunnel, buffer: *mut u8, length: usize) -> usize {
    if handle.is_null() {
        return 0;
    }
    let tunnel = &*handle;
    let inner = lock(&tunnel.inner);
    match inner.as_ref() {
        Some(active) => write_str(&lock(&active.error), buffer, length),
        None => 0,
    }
}

/// Copies the public URL into `buffer` (NUL terminated) and returns its
/// length. Returns 0 until the tunnel is up.
#[no_mangle]
pub unsafe extern "C" fn cft_url(handle: *mut CftTunnel, buffer: *mut u8, length: usize) -> usize {
    if handle.is_null() {
        return 0;
    }
    let tunnel = &*handle;
    let inner = lock(&tunnel.inner);
    match inner.as_ref() {
        Some(active) => write_str(&lock(&active.url), buffer, length),
        None => 0,
    }
}

/// Crate version, for diagnostics.
#[no_mangle]
pub unsafe extern "C" fn cft_version(buffer: *mut u8, length: usize) -> usize {
    write_str(env!("CARGO_PKG_VERSION"), buffer, length)
}

fn set_error(tunnel: &CftTunnel, message: &str) {
    let mut inner = lock(&tunnel.inner);
    if let Some(active) = inner.as_mut() {
        *lock(&active.error) = message.to_owned();
        active.state.store(CFT_ERRORED, Ordering::SeqCst);
    }
}

fn lock<T>(mutex: &Mutex<T>) -> std::sync::MutexGuard<'_, T> {
    mutex.lock().unwrap_or_else(|poisoned| poisoned.into_inner())
}

unsafe fn read_cstr(pointer: *const c_char) -> Result<String, String> {
    if pointer.is_null() {
        return Err("null string".to_owned());
    }
    CStr::from_ptr(pointer)
        .to_str()
        .map(|value| value.to_owned())
        .map_err(|err| format!("invalid UTF-8: {err}"))
}

unsafe fn write_str(value: &str, buffer: *mut u8, length: usize) -> usize {
    if buffer.is_null() || length == 0 {
        return 0;
    }
    let bytes = value.as_bytes();
    let count = bytes.len().min(length - 1);
    std::ptr::copy_nonoverlapping(bytes.as_ptr(), buffer, count);
    *buffer.add(count) = 0;
    count
}

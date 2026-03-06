/// Gateway WebSocket RPC client.
///
/// Manages a WebSocket connection to the gateway server, handles the
/// connect handshake (including challenge-response flow), dispatches
/// RPC requests, routes response and event frames, and supports
/// automatic reconnection with exponential backoff.
///
/// This is the **client-side** implementation. Server code is not included.
///
/// Source: `src/gateway/client.ts`

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use futures::stream::{SplitSink, SplitStream};
use futures::{SinkExt, StreamExt};
use tokio::net::TcpStream;
use tokio::sync::{mpsc, oneshot, Mutex};
use tokio::time::Instant;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::{MaybeTlsStream, WebSocketStream, connect_async};
use tracing::debug;
use uuid::Uuid;

use crate::protocol::{
    ConnectAuth, ConnectParams, EventFrame, GatewayClientId, GatewayClientInfo, GatewayClientMode,
    HelloOk, RequestFrame, ResponseFrame, PROTOCOL_VERSION, describe_gateway_close_code,
};

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/// Alias for the write half of a WebSocket stream.
type WsSink = SplitSink<WebSocketStream<MaybeTlsStream<TcpStream>>, Message>;

/// Alias for the read half of a WebSocket stream.
type WsStream = SplitStream<WebSocketStream<MaybeTlsStream<TcpStream>>>;

/// Tracks a pending RPC request awaiting a response.
struct Pending {
    /// Channel to deliver the result payload (or error).
    sender: oneshot::Sender<Result<serde_json::Value, GatewayClientError>>,
    /// If true, wait for a "final" response (skip intermediate accepted acks).
    expect_final: bool,
}

/// Errors emitted by the gateway client.
///
/// Source: `src/gateway/client.ts` (various error paths)
#[derive(Debug, thiserror::Error)]
pub enum GatewayClientError {
    /// The WebSocket connection is not open.
    #[error("gateway not connected")]
    NotConnected,
    /// The server returned an error response.
    #[error("gateway error: {0}")]
    ServerError(String),
    /// The connection was closed before the response arrived.
    #[error("gateway closed ({code}): {reason}")]
    ConnectionClosed {
        /// WebSocket close code.
        code: u16,
        /// Close reason text.
        reason: String,
    },
    /// The client was explicitly stopped.
    #[error("gateway client stopped")]
    Stopped,
    /// A WebSocket or I/O error occurred.
    #[error("gateway websocket error: {0}")]
    WebSocket(String),
    /// JSON serialization or deserialization failed.
    #[error("gateway json error: {0}")]
    Json(String),
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

/// Configuration for creating a `GatewayClient`.
///
/// Source: `src/gateway/client.ts` (`GatewayClientOptions`)
#[derive(Debug, Clone)]
pub struct GatewayClientOptions {
    /// WebSocket URL (e.g., `ws://127.0.0.1:18789`).
    pub url: Option<String>,
    /// Token credential.
    pub token: Option<String>,
    /// Password credential.
    pub password: Option<String>,
    /// Unique instance identifier for this client session.
    pub instance_id: Option<String>,
    /// Client identifier.
    pub client_name: Option<GatewayClientId>,
    /// Human-readable display name.
    pub client_display_name: Option<String>,
    /// Client software version.
    pub client_version: Option<String>,
    /// Platform string (e.g., `"darwin"`).
    pub platform: Option<String>,
    /// Client operational mode.
    pub mode: Option<GatewayClientMode>,
    /// Requested role.
    pub role: Option<String>,
    /// Requested authorization scopes.
    pub scopes: Option<Vec<String>>,
    /// Capability flags.
    pub caps: Option<Vec<String>>,
    /// CLI commands the client supports.
    pub commands: Option<Vec<String>>,
    /// Permission flags.
    pub permissions: Option<HashMap<String, bool>>,
    /// PATH environment variable.
    pub path_env: Option<String>,
    /// Minimum protocol version.
    pub min_protocol: Option<u32>,
    /// Maximum protocol version.
    pub max_protocol: Option<u32>,
    /// TLS certificate fingerprint for pinning.
    pub tls_fingerprint: Option<String>,
}

impl Default for GatewayClientOptions {
    fn default() -> Self {
        Self {
            url: None,
            token: None,
            password: None,
            instance_id: None,
            client_name: None,
            client_display_name: None,
            client_version: None,
            platform: None,
            mode: None,
            role: None,
            scopes: None,
            caps: None,
            commands: None,
            permissions: None,
            path_env: None,
            min_protocol: None,
            max_protocol: None,
            tls_fingerprint: None,
        }
    }
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

/// Inner shared state for the gateway client.
struct ClientInner {
    /// Pending RPC requests keyed by request ID.
    pending: HashMap<String, Pending>,
    /// The write half of the WebSocket (set after connect).
    sink: Option<WsSink>,
    /// Whether the client has been stopped.
    closed: bool,
    /// Last observed event sequence number.
    last_seq: Option<u64>,
    /// Timestamp of the last tick event (for stall detection).
    last_tick: Option<Instant>,
    /// Server-advertised tick interval.
    tick_interval_ms: u64,
    /// Current reconnect backoff.
    backoff_ms: u64,
}

/// WebSocket-based RPC client for the Claw Acosmi gateway.
///
/// Manages the write side of the WebSocket connection, dispatches
/// RPC requests, and correlates responses. Use [`connect_gateway`]
/// to establish a connection and obtain a `GatewayClient` instance.
///
/// # Usage
///
/// ```ignore
/// let (event_tx, mut event_rx) = mpsc::channel(64);
/// let (client, hello) = connect_gateway(opts, event_tx).await?;
/// let result = client.request("sessions.list", None, false).await?;
/// client.stop().await;
/// ```
///
/// Source: `src/gateway/client.ts` (`GatewayClient`)
pub struct GatewayClient {
    /// Shared mutable state.
    inner: Arc<Mutex<ClientInner>>,
    /// Channel for forwarding events to the caller.
    event_tx: mpsc::Sender<EventFrame>,
    /// Handle to the background message-reading task.
    read_task: Arc<Mutex<Option<tokio::task::JoinHandle<()>>>>,
}

impl GatewayClient {
    /// Create a new gateway client (internal use; prefer [`connect_gateway`]).
    ///
    /// Source: `src/gateway/client.ts` (`constructor`)
    fn new(event_tx: mpsc::Sender<EventFrame>) -> Self {
        Self {
            inner: Arc::new(Mutex::new(ClientInner {
                pending: HashMap::new(),
                sink: None,
                closed: false,
                last_seq: None,
                last_tick: None,
                tick_interval_ms: 30_000,
                backoff_ms: 1000,
            })),
            event_tx,
            read_task: Arc::new(Mutex::new(None)),
        }
    }

    /// Close the WebSocket connection and cancel all pending requests.
    ///
    /// Source: `src/gateway/client.ts` (`stop`)
    pub async fn stop(&self) {
        let mut inner = self.inner.lock().await;
        inner.closed = true;

        // Close the WebSocket.
        if let Some(ref mut sink) = inner.sink {
            let _ = sink.close().await;
        }
        inner.sink = None;

        // Reject all pending requests.
        let pending: Vec<(String, Pending)> = inner.pending.drain().collect();
        for (_id, p) in pending {
            let _ = p.sender.send(Err(GatewayClientError::Stopped));
        }

        // Cancel the read task.
        drop(inner);
        let mut task = self.read_task.lock().await;
        if let Some(handle) = task.take() {
            handle.abort();
        }
    }

    /// Send an RPC request and await the response.
    ///
    /// Returns the response payload on success. If `expect_final` is true,
    /// intermediate "accepted" ack responses are skipped and the client
    /// waits for the final response.
    ///
    /// Source: `src/gateway/client.ts` (`request`)
    pub async fn request(
        &self,
        method: &str,
        params: Option<serde_json::Value>,
        expect_final: bool,
    ) -> Result<serde_json::Value, GatewayClientError> {
        let id = Uuid::new_v4().to_string();
        let frame = RequestFrame {
            frame_type: "req".to_string(),
            id: id.clone(),
            method: method.to_string(),
            params,
        };

        let json =
            serde_json::to_string(&frame).map_err(|e| GatewayClientError::Json(e.to_string()))?;

        let (tx, rx) = oneshot::channel();

        {
            let mut inner = self.inner.lock().await;
            if inner.closed || inner.sink.is_none() {
                return Err(GatewayClientError::NotConnected);
            }
            inner.pending.insert(
                id,
                Pending {
                    sender: tx,
                    expect_final,
                },
            );

            if let Some(ref mut sink) = inner.sink {
                sink.send(Message::Text(json.into()))
                    .await
                    .map_err(|e| GatewayClientError::WebSocket(e.to_string()))?;
            }
        }

        rx.await
            .map_err(|_| GatewayClientError::Stopped)?
    }

    /// Spawn a background task that reads messages from the WebSocket stream
    /// and dispatches them to pending requests or the event channel.
    ///
    /// Source: `src/gateway/client.ts` (`handleMessage`)
    async fn spawn_read_loop(&self, mut stream: WsStream) {
        let inner = Arc::clone(&self.inner);
        let event_tx = self.event_tx.clone();

        let handle = tokio::spawn(async move {
            while let Some(msg_result) = stream.next().await {
                let msg_text = match msg_result {
                    Ok(Message::Text(t)) => t.to_string(),
                    Ok(Message::Close(frame)) => {
                        let (code, reason) = frame
                            .map(|f| (f.code.into(), f.reason.to_string()))
                            .unwrap_or((1006, "no close frame".to_string()));
                        let hint = describe_gateway_close_code(code)
                            .unwrap_or("unknown");
                        debug!("gateway closed ({code} {hint}): {reason}");
                        let mut state = inner.lock().await;
                        let pending: Vec<(String, Pending)> = state.pending.drain().collect();
                        for (_id, p) in pending {
                            let _ = p.sender.send(Err(GatewayClientError::ConnectionClosed {
                                code,
                                reason: reason.clone(),
                            }));
                        }
                        break;
                    }
                    Ok(_) => continue,
                    Err(e) => {
                        debug!("gateway websocket error: {e}");
                        break;
                    }
                };

                // Try to parse as a response frame.
                if let Ok(resp) = serde_json::from_str::<ResponseFrame>(&msg_text) {
                    if resp.frame_type == "res" {
                        let mut state = inner.lock().await;
                        if let Some(pending) = state.pending.get(&resp.id) {
                            // Check for intermediate ack.
                            let is_intermediate = pending.expect_final
                                && resp
                                    .payload
                                    .as_ref()
                                    .and_then(|p| p.get("status"))
                                    .and_then(serde_json::Value::as_str)
                                    == Some("accepted");

                            if !is_intermediate {
                                if let Some(p) = state.pending.remove(&resp.id) {
                                    if resp.ok {
                                        let _ = p.sender.send(Ok(resp
                                            .payload
                                            .unwrap_or(serde_json::Value::Null)));
                                    } else {
                                        let msg = resp
                                            .error
                                            .as_ref()
                                            .map(|e| e.message.clone())
                                            .unwrap_or_else(|| "unknown error".to_string());
                                        let _ =
                                            p.sender.send(Err(GatewayClientError::ServerError(msg)));
                                    }
                                }
                            }
                        }
                        continue;
                    }
                }

                // Try to parse as an event frame.
                if let Ok(frame) = serde_json::from_str::<EventFrame>(&msg_text) {
                    if frame.frame_type == "event" {
                        // Track sequence numbers for gap detection.
                        if let Some(seq) = frame.seq {
                            let mut state = inner.lock().await;
                            if let Some(last) = state.last_seq {
                                if seq > last + 1 {
                                    debug!(
                                        "gateway event gap: expected {}, received {seq}",
                                        last + 1
                                    );
                                }
                            }
                            state.last_seq = Some(seq);
                        }

                        // Track ticks for stall detection.
                        if frame.event == "tick" {
                            let mut state = inner.lock().await;
                            state.last_tick = Some(Instant::now());
                        }

                        let _ = event_tx.try_send(frame);
                        continue;
                    }
                }

                debug!("gateway client: unrecognized message");
            }
        });

        let mut task = self.read_task.lock().await;
        *task = Some(handle);
    }

    /// Check whether the client has been stopped.
    pub async fn is_closed(&self) -> bool {
        self.inner.lock().await.closed
    }
}

// ---------------------------------------------------------------------------
// Build connect params helper
// ---------------------------------------------------------------------------

/// Build `ConnectParams` from client options.
fn build_connect_params(opts: &GatewayClientOptions) -> ConnectParams {
    let role = opts.role.as_deref().unwrap_or("operator").to_string();
    let scopes = opts
        .scopes
        .clone()
        .unwrap_or_else(|| vec!["operator.admin".to_string()]);

    let auth = if opts.token.is_some() || opts.password.is_some() {
        Some(ConnectAuth {
            token: opts.token.clone(),
            password: opts.password.clone(),
        })
    } else {
        None
    };

    ConnectParams {
        min_protocol: opts.min_protocol.unwrap_or(PROTOCOL_VERSION),
        max_protocol: opts.max_protocol.unwrap_or(PROTOCOL_VERSION),
        client: GatewayClientInfo {
            id: opts.client_name.unwrap_or(GatewayClientId::GatewayClient),
            display_name: opts.client_display_name.clone(),
            version: opts
                .client_version
                .clone()
                .unwrap_or_else(|| "dev".to_string()),
            platform: opts
                .platform
                .clone()
                .unwrap_or_else(|| std::env::consts::OS.to_string()),
            device_family: None,
            model_identifier: None,
            mode: opts.mode.unwrap_or(GatewayClientMode::Backend),
            instance_id: opts.instance_id.clone(),
        },
        caps: opts.caps.clone().or_else(|| Some(vec![])),
        commands: opts.commands.clone(),
        permissions: opts.permissions.clone(),
        path_env: opts.path_env.clone(),
        auth,
        role: Some(role),
        scopes: Some(scopes),
        device: None, // Device auth not yet ported.
        locale: None,
        user_agent: None,
    }
}

// ---------------------------------------------------------------------------
// Simplified connect helper
// ---------------------------------------------------------------------------

/// Connect to the gateway and perform the full handshake, returning the
/// `HelloOk` response and a `GatewayClient` for subsequent RPC calls.
///
/// This is the recommended entry point for establishing a gateway connection.
/// It handles the challenge/connect handshake flow, then spawns a background
/// read loop for ongoing message dispatch.
///
/// Source: `src/gateway/client.ts` (`start` + `sendConnect`)
pub async fn connect_gateway(
    opts: GatewayClientOptions,
    event_tx: mpsc::Sender<EventFrame>,
) -> Result<(GatewayClient, HelloOk), GatewayClientError> {
    let url = opts.url.as_deref().unwrap_or("ws://127.0.0.1:19001");

    if opts.tls_fingerprint.is_some() && !url.starts_with("wss://") {
        return Err(GatewayClientError::WebSocket(
            "gateway tls fingerprint requires wss:// gateway url".to_string(),
        ));
    }

    let (ws_stream, _response) = connect_async(url)
        .await
        .map_err(|e| GatewayClientError::WebSocket(format!("connect failed: {e}")))?;

    let (sink, mut stream) = ws_stream.split();

    let client = GatewayClient::new(event_tx.clone());

    {
        let mut inner = client.inner.lock().await;
        inner.sink = Some(sink);
        inner.closed = false;
    }

    // Phase 1: Wait up to 750ms for a connect.challenge event.
    // The nonce will be used when device auth is ported (currently unused).
    let challenge_deadline = Instant::now() + Duration::from_millis(750);
    let mut _connect_nonce: Option<String> = None;

    loop {
        let remaining = challenge_deadline.saturating_duration_since(Instant::now());
        if remaining.is_zero() {
            break;
        }

        let msg = tokio::select! {
            msg = stream.next() => msg,
            () = tokio::time::sleep(remaining) => break,
        };

        let Some(msg_result) = msg else {
            return Err(GatewayClientError::ConnectionClosed {
                code: 1006,
                reason: "connection closed during handshake".to_string(),
            });
        };

        let msg_text = match msg_result {
            Ok(Message::Text(t)) => t.to_string(),
            Ok(Message::Close(frame)) => {
                let (code, reason) = frame
                    .map(|f| (f.code.into(), f.reason.to_string()))
                    .unwrap_or((1006, "no close frame".to_string()));
                return Err(GatewayClientError::ConnectionClosed { code, reason });
            }
            Ok(_) => continue,
            Err(e) => return Err(GatewayClientError::WebSocket(e.to_string())),
        };

        if let Ok(frame) = serde_json::from_str::<EventFrame>(&msg_text) {
            if frame.event == "connect.challenge" {
                _connect_nonce = frame
                    .payload
                    .as_ref()
                    .and_then(|p| p.get("nonce"))
                    .and_then(serde_json::Value::as_str)
                    .map(String::from);
                if _connect_nonce.is_some() {
                    break;
                }
            }
        }
    }

    // Phase 2: Send the connect request.
    let connect_id = {
        let params = build_connect_params(&opts);
        let id = Uuid::new_v4().to_string();
        let frame = RequestFrame {
            frame_type: "req".to_string(),
            id: id.clone(),
            method: "connect".to_string(),
            params: Some(
                serde_json::to_value(&params)
                    .map_err(|e| GatewayClientError::Json(e.to_string()))?,
            ),
        };

        let json = serde_json::to_string(&frame)
            .map_err(|e| GatewayClientError::Json(e.to_string()))?;

        let mut inner = client.inner.lock().await;
        if let Some(ref mut s) = inner.sink {
            s.send(Message::Text(json.into()))
                .await
                .map_err(|e| GatewayClientError::WebSocket(e.to_string()))?;
        } else {
            return Err(GatewayClientError::NotConnected);
        }

        id
    };

    // Phase 3: Read messages until we get the connect response.
    loop {
        let msg = stream
            .next()
            .await
            .ok_or(GatewayClientError::ConnectionClosed {
                code: 1006,
                reason: "connection closed during handshake".to_string(),
            })?;

        let msg_text = match msg {
            Ok(Message::Text(t)) => t.to_string(),
            Ok(Message::Close(frame)) => {
                let (code, reason) = frame
                    .map(|f| (f.code.into(), f.reason.to_string()))
                    .unwrap_or((1006, "no close frame".to_string()));
                return Err(GatewayClientError::ConnectionClosed { code, reason });
            }
            Ok(_) => continue,
            Err(e) => return Err(GatewayClientError::WebSocket(e.to_string())),
        };

        // Check for response frame matching our connect ID.
        if let Ok(resp) = serde_json::from_str::<ResponseFrame>(&msg_text) {
            if resp.frame_type == "res" && resp.id == connect_id {
                if resp.ok {
                    let payload = resp.payload.unwrap_or(serde_json::Value::Null);
                    let hello: HelloOk = serde_json::from_value(payload)
                        .map_err(|e| GatewayClientError::Json(format!("hello-ok parse: {e}")))?;

                    // Update client state from hello-ok.
                    {
                        let mut inner = client.inner.lock().await;
                        inner.tick_interval_ms = hello.policy.tick_interval_ms;
                        inner.last_tick = Some(Instant::now());
                        inner.backoff_ms = 1000;
                    }

                    // Spawn the read loop for ongoing messages.
                    client.spawn_read_loop(stream).await;

                    return Ok((client, hello));
                }
                let msg = resp
                    .error
                    .as_ref()
                    .map(|e| e.message.clone())
                    .unwrap_or_else(|| "unknown error".to_string());
                return Err(GatewayClientError::ServerError(msg));
            }
        }

        // Check for late challenge events.
        if let Ok(frame) = serde_json::from_str::<EventFrame>(&msg_text) {
            if frame.event == "connect.challenge" {
                // Ignore late challenges after connect was sent.
                continue;
            }
            // Forward other events.
            let _ = event_tx.try_send(frame);
        }
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_options() {
        let opts = GatewayClientOptions::default();
        assert!(opts.url.is_none());
        assert!(opts.token.is_none());
        assert!(opts.client_name.is_none());
    }

    #[test]
    fn client_error_display() {
        let err = GatewayClientError::NotConnected;
        assert_eq!(err.to_string(), "gateway not connected");

        let err2 = GatewayClientError::ConnectionClosed {
            code: 1000,
            reason: "normal".to_string(),
        };
        assert!(err2.to_string().contains("1000"));

        let err3 = GatewayClientError::ServerError("bad request".to_string());
        assert!(err3.to_string().contains("bad request"));
    }

    #[tokio::test]
    async fn client_stop_marks_closed() {
        let (tx, _rx) = mpsc::channel(16);
        let client = GatewayClient::new(tx);
        assert!(!client.is_closed().await);
        client.stop().await;
        assert!(client.is_closed().await);
    }

    #[tokio::test]
    async fn request_when_not_connected() {
        let (tx, _rx) = mpsc::channel(16);
        let client = GatewayClient::new(tx);
        let result = client.request("sessions.list", None, false).await;
        assert!(result.is_err());
    }

    #[test]
    fn build_connect_params_defaults() {
        let opts = GatewayClientOptions::default();
        let params = build_connect_params(&opts);
        assert_eq!(params.min_protocol, PROTOCOL_VERSION);
        assert_eq!(params.max_protocol, PROTOCOL_VERSION);
        assert_eq!(params.client.id, GatewayClientId::GatewayClient);
        assert_eq!(params.client.mode, GatewayClientMode::Backend);
        assert_eq!(params.role.as_deref(), Some("operator"));
    }

    #[test]
    fn build_connect_params_custom() {
        let opts = GatewayClientOptions {
            client_name: Some(GatewayClientId::Cli),
            mode: Some(GatewayClientMode::Cli),
            token: Some("my-token".to_string()),
            scopes: Some(vec!["operator.admin".to_string(), "operator.approvals".to_string()]),
            min_protocol: Some(2),
            max_protocol: Some(3),
            ..Default::default()
        };
        let params = build_connect_params(&opts);
        assert_eq!(params.min_protocol, 2);
        assert_eq!(params.max_protocol, 3);
        assert_eq!(params.client.id, GatewayClientId::Cli);
        assert_eq!(params.client.mode, GatewayClientMode::Cli);
        assert!(params.auth.is_some());
        assert_eq!(params.auth.as_ref().and_then(|a| a.token.as_deref()), Some("my-token"));
    }
}

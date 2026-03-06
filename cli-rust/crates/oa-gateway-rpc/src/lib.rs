/// Gateway WebSocket RPC client for Claw Acosmi CLI.
///
/// Handles WebSocket connection to the Gateway service,
/// authentication, protocol framing, network utilities,
/// and RPC call/response patterns.
///
/// Source: `src/gateway/call.ts`, `src/gateway/client.ts`, `src/gateway/auth.ts`,
///         `src/gateway/net.ts`, `src/gateway/protocol/`

/// Gateway RPC protocol types, frame definitions, and constants.
pub mod protocol;

/// Network utilities: LAN IP detection, loopback checks, bind host resolution.
pub mod net;

/// Gateway authentication types and credential resolution.
pub mod auth;

/// Gateway WebSocket client with connect handshake and RPC dispatch.
pub mod client;

/// High-level one-shot gateway RPC call and connection detail resolution.
pub mod call;

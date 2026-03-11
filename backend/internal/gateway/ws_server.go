package gateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/mcpremote"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// ---------- 服务端 WebSocket 处理器 ----------
// 对齐 TS gateway/server-ws-control.ts
// 协议: connect → hello-ok → (request ↔ response)* + event push

// WsServerConfig 服务端 WS 配置。
type WsServerConfig struct {
	Auth           ResolvedGatewayAuth
	TrustedProxies []string
	// AllowedOrigins 额外允许的 Origin 白名单（对齐 TS gateway/origin-check.ts）。
	// 留空时仅凭 CheckBrowserOrigin 内置规则（loopback 双端放行 + requestHost 匹配）。
	AllowedOrigins []string
	State          *GatewayState
	Registry       *MethodRegistry
	SessionStore   *SessionStore
	StorePath      string
	Version        string
	LogFilePath    string
	ConfigLoader   *config.ConfigLoader
	ModelCatalog   *models.ModelCatalog
	// Batch C
	PresenceStore  *SystemPresenceStore
	HeartbeatState *HeartbeatState
	EventQueue     *SystemEventQueue
	// Window 4 — Pipeline dispatcher
	PipelineDispatcher PipelineDispatcher
	// Cron service
	CronService   CronServiceAPI
	CronStorePath string
	// Phase 5: Channel Manager
	ChannelMgr *channels.Manager
	// F3: 可配置握手超时
	HandshakeTimeout time.Duration
	// 技能商店客户端
	SkillStoreClient *skills.SkillStoreClient
	// P2: MCP 远程工具 Bridge
	RemoteMCPBridge *mcpremote.RemoteBridge
	// 启动时间 (用于计算 uptime)
	BootedAt time.Time
	// Phase 5+6: 媒体子系统（可选）
	MediaSubsystem *media.MediaSubsystem
	// 配置提交后的运行态接管
	RestartSentinel  RestartSentinelWriter
	GatewayRestarter GatewayRestarter
	// P1 域隔离: 独立任务运行存储
	TaskStore *TaskStore
}

// HandleWebSocketUpgrade HTTP → WebSocket 升级 + 连接生命周期管理。
func HandleWebSocketUpgrade(cfg WsServerConfig) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		// CheckOrigin 对齐 TS gateway/origin-check.ts checkBrowserOrigin。
		// 内置规则：
		//   1. cfg.AllowedOrigins 白名单精确匹配
		//   2. Origin.host == r.Host 匹配
		//   3. 双端均为 loopback 地址（开发模式本地连接）放行
		// 非浏览器客户端（no Origin header）直接放行，保持向后兼容。
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			// 无 Origin 头：非浏览器客户端（CLI、内部 Go 客户端）直接放行
			if origin == "" {
				return true
			}
			result := CheckBrowserOrigin(r.Host, origin, cfg.AllowedOrigins)
			if !result.OK {
				slog.Warn("ws: origin rejected", "origin", origin, "host", r.Host, "reason", result.Reason)
			}
			return result.OK
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("ws upgrade failed", "error", err)
			return
		}
		conn.SetReadLimit(MaxPayloadBytes)

		slog.Info("ws: new connection", "remote", r.RemoteAddr)
		go wsConnectionLoop(conn, r, cfg)
	}
}

func newGatewayMethodContext(cfg WsServerConfig, liveCfg *types.OpenAcosmiConfig) *GatewayMethodContext {
	return &GatewayMethodContext{
		SessionStore:           cfg.SessionStore,
		StorePath:              cfg.StorePath,
		Config:                 liveCfg,
		LogFilePath:            cfg.LogFilePath,
		ConfigLoader:           cfg.ConfigLoader,
		ModelCatalog:           cfg.ModelCatalog,
		PresenceStore:          cfg.PresenceStore,
		HeartbeatState:         cfg.HeartbeatState,
		EventQueue:             cfg.EventQueue,
		Broadcaster:            cfg.State.Broadcaster(),
		ChatState:              cfg.State.ChatState(),
		PipelineDispatcher:     cfg.PipelineDispatcher,
		EscalationMgr:          cfg.State.EscalationMgr(),
		RemoteApprovalNotifier: cfg.State.RemoteApprovalNotifier(), // P4
		TaskPresetMgr:          cfg.State.TaskPresetMgr(),          // P5
		ChannelMgr:             cfg.ChannelMgr,                     // Phase 5: 频道管理器
		ArgusBridge:            cfg.State.ArgusBridge(),            // Argus 视觉子智能体
		CronService:            cfg.CronService,
		CronStorePath:          cfg.CronStorePath,
		SkillStoreClient:       cfg.SkillStoreClient,             // 技能商店客户端
		RemoteMCPBridge:        cfg.RemoteMCPBridge,              // P2: MCP 远程工具
		UHMSManager:            cfg.State.UHMSManager(),          // P3: UHMS 记忆系统
		UHMSBootMgr:            cfg.State.UHMSBootMgr(),          // Boot 状态管理
		CoderConfirmMgr:        cfg.State.CoderConfirmMgr(),      // Coder 确认流
		PlanConfirmMgr:         cfg.State.PlanConfirmMgr(),       // Phase 1: 方案确认门控
		ResultApprovalMgr:      cfg.State.ResultApprovalMgr(),    // Phase 3: 结果签收门控
		ContractStore:          cfg.State.ContractStore(),        // Phase 8: 合约持久化
		State:                  cfg.State,                        // Phase 4: 子智能体求助通道查找
		MediaSubsystem:         cfg.MediaSubsystem,               // Phase 5+6: 媒体子系统
		ChannelMonitorMgr:      cfg.State.GetChannelMonitorMgr(), // Monitor 频道热更新
		RestartSentinel:        cfg.RestartSentinel,
		GatewayRestarter:       cfg.GatewayRestarter,
		TaskStore:              cfg.TaskStore,
	}
}

// wsConnectionLoop 单连接的读取 + 分发循环。
func wsConnectionLoop(conn *websocket.Conn, r *http.Request, cfg WsServerConfig) {
	connID := uuid.NewString()
	var registered bool
	var writeMu sync.Mutex

	// 关闭清理
	defer func() {
		if registered {
			cfg.State.Broadcaster().RemoveClient(connID)
			cfg.State.Broadcaster().Broadcast("presence.changed", nil, nil)
			slog.Info("ws: client disconnected", "connId", connID)
		}
		conn.Close()
	}()

	// 辅助: 安全写入
	sendRaw := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.TextMessage, data)
	}

	sendJSON := func(v interface{}) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return sendRaw(data)
	}

	// ---------- Phase 0: 发送 connect.challenge ----------
	// 对齐 TS ws-connection.ts: 连接建立后立即发送 nonce
	connectNonce := uuid.NewString()
	challengeEvent := EventFrame{
		Type:  "event",
		Event: "connect.challenge",
		Payload: map[string]interface{}{
			"nonce": connectNonce,
			"ts":    time.Now().UnixMilli(),
		},
	}
	if err := sendJSON(challengeEvent); err != nil {
		slog.Error("ws: failed to send connect.challenge", "error", err)
		return
	}

	// ---------- Phase 1: 等待 connect 帧 ----------
	handshakeTimeout := cfg.HandshakeTimeout
	if handshakeTimeout <= 0 {
		handshakeTimeout = 30 * time.Second
	}
	conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	_, rawMsg, err := conn.ReadMessage()
	if err != nil {
		slog.Warn("ws: failed to read connect frame", "error", err)
		return
	}
	conn.SetReadDeadline(time.Time{}) // 清除 deadline

	// 解析帧类型
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(rawMsg, &rawMap); err != nil {
		sendJSON(ResponseFrame{Type: FrameTypeResponse, OK: false, Error: NewErrorShape(ErrCodeBadRequest, "invalid JSON")})
		return
	}
	frameType := ""
	if raw, ok := rawMap["type"]; ok {
		json.Unmarshal(raw, &frameType)
	}

	// 支持两种 connect 帧格式:
	// 1. 传统格式: {type:"connect", auth:{...}, role:..., ...}
	// 2. UI req 格式: {type:"req", method:"connect", id:"...", params:{auth:{...}, ...}}
	var connectReqID string // 非空时表示 req 格式，需要用 res 帧回复
	var connectRawJSON []byte

	if frameType == FrameTypeConnect {
		// 传统格式: 直接使用整个帧
		connectRawJSON = rawMsg
	} else if frameType == FrameTypeRequest {
		// UI req 格式: 提取 method 和 params
		method := ""
		if raw, ok := rawMap["method"]; ok {
			json.Unmarshal(raw, &method)
		}
		if method != "connect" {
			sendJSON(ResponseFrame{Type: FrameTypeResponse, OK: false, Error: NewErrorShape(ErrCodeBadRequest, "expected connect frame")})
			return
		}
		if raw, ok := rawMap["id"]; ok {
			json.Unmarshal(raw, &connectReqID)
		}
		// 从 params 中提取 connect 参数
		if raw, ok := rawMap["params"]; ok {
			connectRawJSON = raw
		} else {
			connectRawJSON = []byte("{}")
		}
	} else {
		sendJSON(ResponseFrame{Type: FrameTypeResponse, OK: false, Error: NewErrorShape(ErrCodeBadRequest, "expected connect frame")})
		return
	}

	// 解析完整 connect 参数
	var connectParams ConnectParamsFull
	if err := json.Unmarshal(connectRawJSON, &connectParams); err != nil {
		slog.Warn("ws: invalid connect params", "error", err)
		// 允许简化 connect（只有 role+scopes），不阻塞
	}

	// 设置默认 role
	role := connectParams.Role
	if role == "" {
		role = "operator"
	}

	// F3: 协议版本协商 — 对齐 TS message-handler.ts L312-337
	// 检查服务端 ProtocolVersion 是否在客户端 [minProtocol, maxProtocol] 范围内
	clientMin := connectParams.MinProtocol
	clientMax := connectParams.MaxProtocol
	if clientMax > 0 && (clientMax < ProtocolVersion || clientMin > ProtocolVersion) {
		closeMsg := fmt.Sprintf("protocol mismatch: server=%d, client=[%d,%d]",
			ProtocolVersion, clientMin, clientMax)
		sendJSON(map[string]interface{}{
			"type":  "error",
			"error": NewErrorShape(ErrCodeProtocolMismatch, closeMsg),
		})
		conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(1002, "protocol mismatch"),
			time.Now().Add(5*time.Second),
		)
		slog.Warn("ws: protocol mismatch", "connId", connID,
			"serverVersion", ProtocolVersion,
			"clientMin", clientMin, "clientMax", clientMax)
		return
	}

	// 判断是否为本地客户端
	isLocalClient := isLocalAddr(r.RemoteAddr)

	// ---------- Phase 1.5: nonce 验证 ----------
	// 对齐 TS message-handler.ts: 非本地连接且有 device 块时验证 nonce
	if connectParams.Device != nil && connectParams.Device.Nonce != "" {
		if connectParams.Device.Nonce != connectNonce {
			closeMsg := "device nonce mismatch"
			sendJSON(map[string]interface{}{
				"type":  "error",
				"error": NewErrorShape(ErrCodeUnauthorized, closeMsg),
			})
			conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(1008, closeMsg),
				time.Now().Add(5*time.Second),
			)
			slog.Warn("ws: nonce mismatch", "connId", connID)
			return
		}
	}

	// ---------- Phase 2: 认证 ----------
	var connectAuth *ConnectAuth
	if connectParams.Auth != nil {
		connectAuth = &ConnectAuth{
			Token:    connectParams.Auth.Token,
			Password: connectParams.Auth.Password,
		}
	}
	authResult := AuthorizeGatewayConnect(AuthorizeParams{
		Auth:           cfg.Auth,
		ConnectAuth:    connectAuth,
		Req:            r,
		TrustedProxies: cfg.TrustedProxies,
	})
	if !authResult.OK {
		closeMsg := "authentication failed"
		if authResult.Reason != "" {
			closeMsg = authResult.Reason
		}
		sendJSON(map[string]interface{}{
			"type":  "error",
			"error": NewErrorShape(ErrCodeUnauthorized, closeMsg),
		})
		// 对齐 TS message-handler.ts: 认证失败使用 1008 (policy violation)
		conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(1008, closeMsg),
			time.Now().Add(5*time.Second),
		)
		return
	}

	// ---------- Phase 2.5: 设备认证 ----------
	// 对齐 TS message-handler.ts L401-659: 验证设备签名
	var clientID, clientMode string
	if connectParams.Client.ID != "" {
		clientID = connectParams.Client.ID
	}
	if connectParams.Client.Mode != "" {
		clientMode = connectParams.Client.Mode
	}
	authToken := ""
	if connectParams.Auth != nil {
		authToken = connectParams.Auth.Token
	}
	deviceAuthResult := ValidateDeviceAuth(
		connectParams.Device,
		clientID, clientMode, role, connectParams.Scopes,
		authToken, isLocalClient,
	)
	if !deviceAuthResult.OK {
		closeMsg := deviceAuthResult.Reason
		sendJSON(map[string]interface{}{
			"type":  "error",
			"error": NewErrorShape(ErrCodeUnauthorized, closeMsg),
		})
		conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(1008, closeMsg),
			time.Now().Add(5*time.Second),
		)
		slog.Warn("ws: device auth failed", "connId", connID, "reason", closeMsg)
		return
	}

	// ---------- Phase 3: 发送 hello-ok ----------
	methods := cfg.Registry.Methods()
	helloOk := HelloOk{
		Type:     FrameTypeHelloOk,
		Protocol: ProtocolVersion,
		Server: HelloOkServer{
			Version: cfg.Version,
			ConnID:  connID,
		},
		Features: HelloOkFeatures{
			Methods: methods,
			Events:  defaultGatewayEvents(),
		},
		Policy: DefaultHelloOkPolicy(),
		Snapshot: &SnapshotData{
			UptimeMs: time.Since(cfg.BootedAt).Milliseconds(),
			StateVersion: &SnapshotStateVersion{
				Presence: 0,
				Health:   0,
			},
		},
	}
	// 根据 connect 帧格式选择响应方式
	if connectReqID != "" {
		// UI req 格式: 用 {type:"res", id, ok, payload} 包裹
		resFrame := ResponseFrame{
			Type:    FrameTypeResponse,
			ID:      connectReqID,
			OK:      true,
			Payload: helloOk,
		}
		if err := sendJSON(resFrame); err != nil {
			slog.Error("ws: failed to send hello-ok (res)", "error", err)
			return
		}
	} else {
		// 传统格式: 直接发送 hello-ok
		if err := sendJSON(helloOk); err != nil {
			slog.Error("ws: failed to send hello-ok", "error", err)
			return
		}
	}
	slog.Info("ws: hello-ok sent", "connId", connID, "role", role)

	// ---------- Phase 4: 注册到 Broadcaster ----------
	client := &WsClient{
		ConnID:         connID,
		Connect:        ConnectParams{Role: role, Scopes: connectParams.Scopes},
		ClientIP:       ResolveGatewayClientIP(r.RemoteAddr, GetHeader(r, "X-Forwarded-For"), GetHeader(r, "X-Real-IP"), cfg.TrustedProxies),
		Send:           sendRaw,
		Close:          func(code int, reason string) error { return conn.Close() },
		BufferedAmount: func() int64 { return 0 }, // 写超时已兜底，不再用累计字节
	}
	cfg.State.Broadcaster().AddClient(client)
	cfg.State.Broadcaster().Broadcast("presence.changed", nil, nil)
	registered = true

	// ---------- Phase 5: 请求-响应循环 ----------
	// ping 保活
	ticker := time.NewTicker(30 * time.Second)
	pingDone := make(chan struct{})
	defer func() {
		ticker.Stop()
		close(pingDone) // 通知 goroutine 退出
	}()

	go func() {
		for {
			select {
			case <-pingDone:
				return
			case <-ticker.C:
				writeMu.Lock()
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					writeMu.Unlock()
					return
				}
				writeMu.Unlock()
			}
		}
	}()

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})
	conn.SetReadDeadline(time.Now().Add(90 * time.Second))

	for {
		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("ws: unexpected close", "connId", connID, "error", err)
			}
			return
		}

		// 解析请求帧
		var reqFrame RequestFrame
		if err := json.Unmarshal(rawMsg, &reqFrame); err != nil {
			sendJSON(ResponseFrame{
				Type:  FrameTypeResponse,
				OK:    false,
				Error: NewErrorShape(ErrCodeBadRequest, "invalid request frame"),
			})
			continue
		}

		if reqFrame.Type != FrameTypeRequest {
			continue // 忽略非 request 帧
		}

		if reqFrame.ID == "" || reqFrame.Method == "" {
			sendJSON(ResponseFrame{
				Type:  FrameTypeResponse,
				ID:    reqFrame.ID,
				OK:    false,
				Error: NewErrorShape(ErrCodeBadRequest, "missing id or method"),
			})
			continue
		}

		var liveCfg *types.OpenAcosmiConfig
		if cfg.ConfigLoader != nil {
			if currentCfg, err := cfg.ConfigLoader.LoadConfig(); err == nil {
				liveCfg = currentCfg
			}
		}

		// 构建 GatewayClient (对齐 server_methods.go GatewayClient 类型)
		gatewayClient := &GatewayClient{
			ConnID: connID,
			Connect: &ConnectParamsFull{
				Role:   role,
				Scopes: connectParams.Scopes,
			},
		}

		// 构建方法上下文
		methodCtx := newGatewayMethodContext(cfg, liveCfg)

		// 创建同步 respond 回调
		respond := func(ok bool, payload interface{}, errShape *ErrorShape) {
			resp := ResponseFrame{
				Type:    FrameTypeResponse,
				ID:      reqFrame.ID,
				OK:      ok,
				Payload: payload,
				Error:   errShape,
			}
			if sendErr := sendJSON(resp); sendErr != nil {
				slog.Warn("ws: failed to send response",
					"connId", connID, "method", reqFrame.Method, "error", sendErr)
			}
		}

		// 分发到 MethodRegistry
		HandleGatewayRequest(cfg.Registry, &reqFrame, gatewayClient, methodCtx, respond)
	}
}

// defaultGatewayEvents 返回网关支持的事件列表。
func defaultGatewayEvents() []string {
	return []string{
		"chat.delta",
		"chat.final",
		"chat.error",
		"chat.abort",
		"chat.tool",
		"sessions.changed",
		"presence.changed",
		"health.changed",
		"voice.wake.changed",
		"exec.approval.requested",
		"exec.approval.resolved",
		"gateway.tick",
		"gateway.shutdown",
		"argus.status.changed",
		// 任务看板事件（Phase 1: 主动消息推送）
		EventTaskQueued,
		EventTaskStarted,
		EventTaskProgress,
		EventTaskCompleted,
		EventTaskFailed,
	}
}

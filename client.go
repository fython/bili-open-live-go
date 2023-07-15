package biliopen

import (
	"bytes"
	"context"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"io"
	"net/http"
	"nhooyr.io/websocket"
	"sync"
	"time"
)

// clientState 直播客户端状态
type clientState int

const (
	clientStateIdle clientState = iota
	clientStateActive
)

// noCopy may be embedded into structs which must not be copied
// after the first use.
//
// See https://golang.org/issues/8005#issuecomment-190753527
// for details.
type noCopy struct{}

// Lock is a no-op used by -copylocks checker from `go vet`.
func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// LiveClient 直播 API 客户端实现
//
// 协议根据官方开发文档实现：https://open-live.bilibili.com/document/74eec767-e594-7ddd-6aba-257e8317c05d
type LiveClient struct {
	ApiHost   string
	AppKey    string
	AppSecret string
	ProjectID int64

	OnDanmaku func(Danmaku)
	OnClose   func(error)

	noCopy noCopy

	mu          sync.Mutex
	client      *http.Client
	clientState clientState
	liveCode    string
	gameID      string
	wsInfo      websocketInfo
	wsClient    *liveWebsocketClient
}

func (c *LiveClient) getApiHost() string {
	host := c.ApiHost
	if host == "" {
		host = ApiHostRelease
	}
	return host
}

func (c *LiveClient) logger() *zap.Logger {
	return zap.L().With(zap.String("logger", "LiveClient"))
}

// Connect 建立直播间连接
//
// 需要传入主播自己的身份码，而不是直播间 ID，遂不支持监听其他人的直播间
func (c *LiveClient) Connect(ctx context.Context, liveCode string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.clientState != clientStateIdle {
		return fmt.Errorf("client state should be idle")
	}
	c.liveCode = liveCode
	c.client = &http.Client{
		Timeout:   time.Second * 60,
		Transport: ApiTransport{AppKey: c.AppKey, AppSecret: c.AppSecret},
	}
	// 调用 /v2/app/start 获取基本信息
	if err := c.callAppStart(ctx); err != nil {
		return fmt.Errorf("start app fail: %w", err)
	}
	c.clientState = clientStateActive
	// 拿到基本信息后，自动建立 WebSocket 连接
	if err := c.connectWs(ctx); err != nil {
		return fmt.Errorf("connect ws fail: %w", err)
	}
	return nil
}

// connectWs 连接 WebSocket
func (c *LiveClient) connectWs(ctx context.Context) error {
	if c.wsClient != nil {
		if err := c.wsClient.Close(); err != nil {
			c.logger().Warn("close last websocket client fail", zap.Error(err))
		}
	}
	// 创建新的 WebSocket 连接客户端
	c.wsClient = &liveWebsocketClient{
		url:       c.wsInfo.WSSLink[0],
		authBody:  c.wsInfo.AuthBody,
		onDanmaku: c.OnDanmaku,
		onClose:   c.onWsClose,
	}
	if err := c.wsClient.connect(ctx); err != nil {
		c.logger().Error("connect websocket fail", zap.Error(err),
			zap.String("url", c.wsClient.url), zap.String("auth_body", c.wsClient.authBody))
		return fmt.Errorf("connect websocket fail: %w", err)
	}
	return nil
}

// onWsClose 在 WebSocket 连接断线的时候一起触发 Disconnect 函数
func (c *LiveClient) onWsClose(err error) {
	if err := c.Disconnect(context.Background()); err != nil {
		c.logger().Warn("disconnect fail", zap.Error(err))
	}
	if c.OnClose != nil {
		c.OnClose(err)
	}
}

// Disconnect 断开连接
func (c *LiveClient) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() {
		c.clientState = clientStateIdle
	}()
	if c.clientState == clientStateActive {
		if err := c.callAppEnd(ctx); err != nil {
			c.logger().Warn("call app end fail", zap.Error(err))
		}
	}
	if c.wsClient != nil {
		defer func() {
			c.wsClient = nil
		}()
		if err := c.wsClient.Close(); err != nil {
			c.logger().Warn("close last websocket client fail", zap.Error(err))
		}
	}
	return nil
}

// IsActive 检查客户端是否在线
func (c *LiveClient) IsActive() bool {
	return c.clientState == clientStateActive
}

func (c *LiveClient) commonCallApi(ctx context.Context, path string, req any, rsp any) error {
	reqJson, err := jsoniter.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal fail: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.getApiHost()+path, bytes.NewReader(reqJson))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpRsp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do http request fail: %w", err)
	}
	if httpRsp.StatusCode != http.StatusOK {
		return fmt.Errorf("http response is not ok: status code %d", httpRsp.StatusCode)
	}
	defer httpRsp.Body.Close()
	rspBytes, err := io.ReadAll(httpRsp.Body)
	if err != nil {
		return fmt.Errorf("read body fail: %w", err)
	}
	if err = jsoniter.Unmarshal(rspBytes, rsp); err != nil {
		return fmt.Errorf("unmarshal response fail: %w", err)
	}
	return nil
}

// callAppStart 开启游戏/项目，获取 WebSocket 连接节点和鉴权信息
func (c *LiveClient) callAppStart(ctx context.Context) error {
	req := map[string]any{"code": c.liveCode, "app_id": c.ProjectID}
	var rsp CommonResponse[appStartData]
	if err := c.commonCallApi(ctx, "/v2/app/start", req, &rsp); err != nil {
		return err
	}
	if err := rsp.Err(); err != nil {
		return err
	}
	c.gameID = rsp.Data.GameInfo.GameID
	c.wsInfo = rsp.Data.WebsocketInfo
	return nil
}

// callAppEnd 关闭游戏/项目，对于游戏类型的项目必须要调用这个，否则下次无法开启
func (c *LiveClient) callAppEnd(ctx context.Context) error {
	if c.clientState != clientStateActive {
		return fmt.Errorf("client state should be alive")
	}
	if c.gameID == "" {
		// 一些直播应用会拿不到 Game ID，此时无需手动结束
		return nil
	}
	req := map[string]any{"app_id": c.ProjectID, "game_id": c.gameID}
	var rsp CommonResponse[any]
	if err := c.commonCallApi(ctx, "/v2/app/end", req, &rsp); err != nil {
		return err
	}
	if err := rsp.Err(); err != nil {
		return err
	}
	return nil
}

// callAppHeartbeat 发送心跳包
func (c *LiveClient) callAppHeartbeat(ctx context.Context) error {
	if c.clientState != clientStateActive {
		return fmt.Errorf("client state should be alive")
	}
	if c.gameID == "" {
		// 一些直播应用会拿不到 Game ID，此时无需触发心跳
		return nil
	}
	req := map[string]any{"game_id": c.gameID}
	var rsp CommonResponse[any]
	if err := c.commonCallApi(ctx, "/v2/app/heartbeat", req, &rsp); err != nil {
		return err
	}
	if err := rsp.Err(); err != nil {
		return err
	}
	return nil
}

// websocketClientState WebSocket 客户端状态
type websocketClientState int

const (
	// websocketClientStateIdle 闲置，未连接
	websocketClientStateIdle websocketClientState = iota
	// websocketClientStateAuth 需要登录，可能正在登录中
	websocketClientStateAuth
	// websocketClientStateActive 已连接
	websocketClientStateActive
)

// liveWebsocketClient 封装长连 Websocket 协议的客户端
type liveWebsocketClient struct {
	url       string
	authBody  string
	onDanmaku func(Danmaku)
	onClose   func(error)

	state           websocketClientState
	conn            *websocket.Conn
	seqID           int32
	heartbeatTicker *time.Ticker
	loopCtx         context.Context
	loopCancel      func()

	eventCh      chan *wsProtoMsg
	eventHandler map[wsProtoOp]func(*wsProtoMsg) error
}

func (c *liveWebsocketClient) logger() *zap.Logger {
	return zap.L().With(zap.String("logger", "liveWebsocketClient"))
}

func (c *liveWebsocketClient) connect(ctx context.Context) error {
	if c.state != websocketClientStateIdle {
		return fmt.Errorf("websocket client state should be idle")
	}
	conn, _, err := websocket.Dial(ctx, c.url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"User-Agent": []string{"bili-open-live-go/1.0"},
		},
	})
	if err != nil {
		return fmt.Errorf("dial fail: %w", err)
	}
	c.conn = conn

	// init states
	c.eventCh = make(chan *wsProtoMsg)
	c.eventHandler = map[wsProtoOp]func(*wsProtoMsg) error{
		wsProtoOpAuthReply:      c.handleOpAuth,
		wsProtoOpHeartbeatReply: c.handleOpHeartbeat,
		wsProtoOpSendMsgReply:   c.handleOpMsg,
	}
	c.seqID = 0
	c.state = websocketClientStateAuth

	// init loops
	c.loopCtx, c.loopCancel = context.WithCancel(context.Background())
	c.heartbeatTicker = time.NewTicker(time.Second * 5)
	go c.readLoop()
	go c.eventLoop()

	// start auth
	if err = c.sendAuth(); err != nil {
		return fmt.Errorf("send auth fail: %w", err)
	}

	return nil
}

// Close 主动关闭连接
func (c *liveWebsocketClient) Close() error {
	if c.conn == nil {
		return nil
	}
	defer c.internalClose(nil)
	if err := c.conn.Close(websocket.StatusNormalClosure, "client close"); err != nil {
		return err
	}
	return nil
}

// internalClose 回收连接相关的状态、上下文，并通知 onClose 回调，若为主动关闭则传入空失败区分
func (c *liveWebsocketClient) internalClose(err error) {
	if c.loopCancel != nil {
		c.loopCancel()
		c.loopCancel = nil
	}
	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
		c.heartbeatTicker = nil
	}
	c.state = websocketClientStateIdle
	c.conn = nil
	if c.onClose != nil {
		c.onClose(err)
	}
}

// readLoop WebSocket 数据流读取循环，反序列化出接口消息写入 channel 队列待处理
func (c *liveWebsocketClient) readLoop() {
	for {
		if c.conn == nil {
			c.logger().Info("connection is closed. exit read loop")
			return
		}
		_, buf, err := c.conn.Read(context.Background())
		if err != nil {
			if closeStatus := websocket.CloseStatus(err); closeStatus != -1 {
				c.logger().Info("connection receive close message", zap.Error(err))
				c.internalClose(err)
				return
			}
			c.logger().Warn("failed to read message from conn", zap.Error(err))
			continue
		}
		msg, err := parseWsProtoMsg(buf)
		if err != nil {
			c.logger().Warn("failed to parse message", zap.Error(err))
			continue
		}
		c.logger().Debug("recv msg", zap.Any("msg", msg))
		c.eventCh <- msg
	}
}

// eventLoop 接口消息消费循环
func (c *liveWebsocketClient) eventLoop() {
	for {
		select {
		case <-c.loopCtx.Done():
			return
		case <-c.heartbeatTicker.C:
			if err := c.sendHeartbeat(); err != nil {
				c.logger().Warn("heartbeat send fail", zap.Error(err))
			}
		case msg := <-c.eventCh:
			if msg == nil {
				continue
			}
			handler, ok := c.eventHandler[msg.Operation]
			if !ok {
				c.logger().Warn("no handlers for this message", zap.Int32("operation", int32(msg.Operation)))
				continue
			}
			if err := handler(msg); err != nil {
				c.logger().Warn("handle msg fail", zap.Error(err))
			}
		}
	}
}

func (c *liveWebsocketClient) createMsg(op wsProtoOp, body []byte) *wsProtoMsg {
	msg := &wsProtoMsg{
		Operation:  op,
		SequenceID: c.seqID,
		Body:       body,
	}
	c.seqID++
	return msg
}

func (c *liveWebsocketClient) writeMsg(msg *wsProtoMsg) error {
	w, err := c.conn.Writer(context.Background(), websocket.MessageBinary)
	if err != nil {
		return fmt.Errorf("open writer fail: %w", err)
	}
	defer w.Close()
	return writeWsProtoMsg(w, msg)
}

func (c *liveWebsocketClient) sendHeartbeat() error {
	if c.state != websocketClientStateActive {
		return nil
	}
	msg := c.createMsg(wsProtoOpHeartbeat, nil)
	return c.writeMsg(msg)
}

func (c *liveWebsocketClient) sendAuth() error {
	if c.state != websocketClientStateAuth {
		return nil
	}
	msg := c.createMsg(wsProtoOpAuth, []byte(c.authBody))
	return c.writeMsg(msg)
}

func (c *liveWebsocketClient) handleOpAuth(msg *wsProtoMsg) error {
	if c.state != websocketClientStateAuth {
		return fmt.Errorf("receive op msg at unexpected state %d", c.state)
	}
	var rsp wsAuthResponse
	if err := jsoniter.Unmarshal(msg.Body, &rsp); err != nil {
		return fmt.Errorf("unmarshal auth response fail: %w", err)
	}
	if rsp.Code != 0 {
		return fmt.Errorf("auth response code is not zero: %d", rsp.Code)
	}
	c.state = websocketClientStateActive
	c.logger().Info("client finish auth")
	return nil
}

func (c *liveWebsocketClient) handleOpHeartbeat(msg *wsProtoMsg) error {
	c.logger().Info("op heartbeat", zap.String("msg.body", string(msg.Body)))
	return nil
}

func (c *liveWebsocketClient) handleOpMsg(msg *wsProtoMsg) error {
	cmd := jsoniter.Get(msg.Body, "cmd").ToString()
	if cmd == CmdLiveOpenPlatformDm {
		var dm Danmaku
		dataNode := jsoniter.Get(msg.Body, "data")
		dataNode.ToVal(&dm)
		if err := dataNode.LastError(); err != nil {
			return fmt.Errorf("unmarshal danmaku fail: %w", err)
		}
		if c.onDanmaku != nil {
			c.onDanmaku(dm)
		}
	} else {
		c.logger().Warn("unsupported cmd", zap.String("cmd", cmd), zap.String("msg", string(msg.Body)))
	}

	return nil
}

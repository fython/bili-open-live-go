package biliopen

import (
	"encoding/binary"
	"fmt"
	"io"
)

// WebSocket 协议文档见 https://open-live.bilibili.com/document/657d8e34-f926-a133-16c0-300c1afc6e6b
// 此文件参考了官方 Go 样例实现，对代码风格进行了部分修改

const (
	wsProtoMaxBodySize     = int32(1 << 11)
	wsProtoCmdSize         = 4
	wsProtoPackSize        = 4
	wsProtoHeaderSize      = 2
	wsProtoVerSize         = 2
	wsProtoOperationSize   = 4
	wsProtoSeqIdSize       = 4
	wsProtoHeartbeatSize   = 4
	wsProtoRawHeaderSize   = wsProtoPackSize + wsProtoHeaderSize + wsProtoVerSize + wsProtoOperationSize + wsProtoSeqIdSize
	wsProtoMaxPackSize     = wsProtoMaxBodySize + int32(wsProtoRawHeaderSize)
	wsProtoPackOffset      = 0
	wsProtoHeaderOffset    = wsProtoPackOffset + wsProtoPackSize
	wsProtoVerOffset       = wsProtoHeaderOffset + wsProtoHeaderSize
	wsProtoOperationOffset = wsProtoVerOffset + wsProtoVerSize
	wsProtoSeqIdOffset     = wsProtoOperationOffset + wsProtoOperationSize
	wsProtoHeartbeatOffset = wsProtoSeqIdOffset + wsProtoSeqIdSize
)

// wsProtoOp WebSocket 协议消息类型
type wsProtoOp int32

// WebSocket 协议消息类型枚举列表，以 Reply 结尾的类型通常由服务端发送回来
const (
	wsProtoOpHeartbeat      wsProtoOp = 2
	wsProtoOpHeartbeatReply wsProtoOp = 3
	wsProtoOpSendMsgReply   wsProtoOp = 5
	wsProtoOpAuth           wsProtoOp = 7
	wsProtoOpAuthReply      wsProtoOp = 8
)

// wsProtoMsg WebSocket 协议消息体
// 具体序列化过程见 writeWsProtoMsg 和 parseWsProtoMsg
type wsProtoMsg struct {
	Version    int16
	Operation  wsProtoOp
	SequenceID int32
	Body       []byte
}

// writeWsProtoMsg 将 wsProtoMsg 序列化数据写入 io.Writer 中
func writeWsProtoMsg(w io.Writer, p *wsProtoMsg) error {
	packSize := int32(wsProtoRawHeaderSize + len(p.Body))
	data := []any{
		packSize,
		int16(wsProtoRawHeaderSize),
		p.Version,
		int32(p.Operation),
		p.SequenceID,
		p.Body,
	}
	for _, d := range data {
		if err := binary.Write(w, binary.BigEndian, d); err != nil {
			return err
		}
	}
	return nil
}

// parseWsProtoMsg 从 []byte 中反序列化 wsProtoMsg
func parseWsProtoMsg(buf []byte) (p *wsProtoMsg, err error) {
	p = new(wsProtoMsg)
	packSize := int32(binary.BigEndian.Uint32(buf[wsProtoPackOffset:wsProtoHeaderOffset]))
	headerLength := int16(binary.BigEndian.Uint16(buf[wsProtoHeaderOffset:wsProtoVerOffset]))
	p.Version = int16(binary.BigEndian.Uint16(buf[wsProtoVerOffset:wsProtoOperationOffset]))
	p.Operation = wsProtoOp(binary.BigEndian.Uint32(buf[wsProtoOperationOffset:wsProtoSeqIdOffset]))
	p.SequenceID = int32(binary.BigEndian.Uint32(buf[wsProtoSeqIdOffset:]))
	if packSize < 0 || packSize > wsProtoMaxPackSize {
		return p, fmt.Errorf("invalid pack size: %d", packSize)
	}
	if len(buf) < int(packSize) {
		return p, fmt.Errorf("buffer length %d is smaller than packet size %d", len(buf), packSize)
	}
	if headerLength != wsProtoRawHeaderSize {
		return p, fmt.Errorf("unsupported header size: %d", headerLength)
	}
	bodySize := int(packSize - int32(headerLength))
	if bodySize <= 0 {
		return p, fmt.Errorf("invalid body size: %d", bodySize)
	}
	p.Body = buf[headerLength:packSize]
	return p, nil
}

// wsAuthResponse WebSocket 协议登录结果
type wsAuthResponse struct {
	Code int64 `json:"code"`
}

const (
	// CmdLiveOpenPlatformDm 在 Websocket 协议中接收到的消息类型：开放平台弹幕，目前只实现了这个类型
	CmdLiveOpenPlatformDm = "LIVE_OPEN_PLATFORM_DM"
)

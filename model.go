package biliopen

// TODO 需要对齐 https://open-live.bilibili.com/document/f9ce25be-312e-1f4a-85fd-fef21f1637f8 模型定义

// Danmaku 弹幕信息
type Danmaku struct {
	// Timestamp 时间戳
	Timestamp int `json:"timestamp"`
	// RoomID 直播间 ID
	RoomID int `json:"room_id"`

	// UID 用户 UID
	UID int `json:"uid"`
	// Username 用户名
	Username string `json:"uname"`
	// UserFace 用户头像
	UserFace string `json:"uface"`
	// Admin 是否房管
	Admin int `json:"admin"`
	// Vip 是否月费会员
	Vip int `json:"vip"`
	// SVip 是否年费会员
	SVip int `json:"svip"`

	// MsgType 是否礼物弹幕（节奏风暴）
	MsgType int `json:"msg_type"`
	// DMType 弹幕类型，枚举值参考 DanmakuType 类型常量
	DMType DanmakuType `json:"dm_type"`

	// Message 弹幕内容
	Message string `json:"msg"`
	// MessageID 弹幕 ID，猜测用于去重
	MessageID string `json:"msg_id"`
	// EmojiImgUrl 表情地址
	EmojiImgUrl string `json:"emoji_img_url"`

	// FansMedalLevel 粉丝牌等级
	FansMedalLevel int `json:"fans_medal_level"`
	// FansMedalName 粉丝牌名称
	FansMedalName string `json:"fans_medal_name"`
	// FansMedalWearingStatus 粉丝牌是否穿戴
	FansMedalWearingStatus bool `json:"fans_medal_wearing_status"`
}

// DanmakuType 弹幕类型
type DanmakuType int

const (
	// DanmakuTypeText 文字
	DanmakuTypeText DanmakuType = 0
	// DanmakuTypeSticker 表情
	DanmakuTypeSticker DanmakuType = 1
	// DanmakuTypeVoice 语音
	DanmakuTypeVoice DanmakuType = 2
)

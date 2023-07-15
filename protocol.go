package biliopen

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	ApiHostRelease = "https://live-open.biliapi.com"
)

const (
	HeaderBiliCommPrefix       = "X-Bili-"
	HeaderBiliContentMD5       = HeaderBiliCommPrefix + "Content-MD5"
	HeaderBiliTimestamp        = HeaderBiliCommPrefix + "Timestamp"
	HeaderBiliSignatureMethod  = HeaderBiliCommPrefix + "Signature-Method"
	HeaderBiliSignatureNonce   = HeaderBiliCommPrefix + "Signature-Nonce"
	HeaderBiliAccessKeyId      = HeaderBiliCommPrefix + "AccessKeyId"
	HeaderBiliSignatureVersion = HeaderBiliCommPrefix + "Signature-Version"
)

// signatureSourceHeaders header keys should be ordered by alphabetical in code
var signatureSourceHeaders = []string{
	HeaderBiliAccessKeyId,
	HeaderBiliContentMD5,
	HeaderBiliSignatureMethod,
	HeaderBiliSignatureNonce,
	HeaderBiliSignatureVersion,
	HeaderBiliTimestamp,
}

func GenerateSignature(appSecret string, header http.Header) string {
	var buf bytes.Buffer
	for _, headerKey := range signatureSourceHeaders {
		buf.WriteString(strings.ToLower(headerKey))
		buf.WriteByte(':')
		buf.WriteString(header.Get(headerKey))
		buf.WriteByte('\n')
	}
	s := buf.Bytes()
	s = s[:len(s)-1]
	hm := hmac.New(sha256.New, []byte(appSecret))
	hm.Write(s)
	return hex.EncodeToString(hm.Sum(nil))
}

// ApiTransport implements bili open api http transport with auto signature
//
// these request headers will be generated in RoundTrip method:
// - X-Bili-Timestamp
// - X-Bili-Signature-Method
// - X-Bili-Content-MD5
// - Authorization
type ApiTransport struct {
	AppKey    string
	AppSecret string
	Transport http.RoundTripper
}

func (t ApiTransport) RoundTrip(r *http.Request) (rsp *http.Response, err error) {
	r = r.Clone(r.Context())

	// generate base api header
	ts := time.Now().Unix()
	nonce := fmt.Sprintf("%d%08d", ts, rand.Intn(10e8))
	r.Header.Set(HeaderBiliTimestamp, strconv.FormatInt(ts, 10))
	r.Header.Set(HeaderBiliSignatureMethod, "HMAC-SHA256")
	r.Header.Set(HeaderBiliSignatureNonce, nonce)
	r.Header.Set(HeaderBiliAccessKeyId, t.AppKey)
	r.Header.Set(HeaderBiliSignatureVersion, "1.0")
	// generate content md5
	if r.Method == http.MethodPost {
		var body io.ReadCloser
		body, err = r.GetBody()
		if err != nil {
			return nil, fmt.Errorf("failed to get body: %w", err)
		}
		defer body.Close()
		bodyBytes, _ := io.ReadAll(body)
		m := md5.New()
		m.Write(bodyBytes)
		r.Header.Set(HeaderBiliContentMD5, hex.EncodeToString(m.Sum(nil)))
	}
	// generate api signature depends on header values before this line
	sign := GenerateSignature(t.AppSecret, r.Header)
	r.Header.Set("Authorization", sign)

	log.Printf("%+v", r.Header)

	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(r)
}

type CommonResponse[T any] struct {
	Code      CommonErrorCode `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
	Data      T               `json:"data"`
}

func (r CommonResponse[T]) Err() error {
	if r.Code != 0 {
		return CommonError{Code: r.Code, Message: r.Message, RequestID: r.RequestID}
	}
	return nil
}

func (r CommonResponse[T]) String() string {
	var buf bytes.Buffer
	buf.WriteString("CommonResponse[")
	buf.WriteString(r.RequestID)
	buf.WriteString("]{code=")
	buf.WriteString(strconv.Itoa(int(r.Code)))
	buf.WriteString(", message=")
	buf.WriteString(r.Message)
	if r.Code == 0 {
		buf.WriteString(", data=")
		buf.WriteString(fmt.Sprint(r.Data))
	}
	buf.WriteString("}")
	return buf.String()
}

type appStartData struct {
	GameInfo      appStartGameInfo `json:"game_info"`
	WebsocketInfo websocketInfo    `json:"websocket_info"`
	AnchorInfo    anchorInfo       `json:"anchor_info"`
}

type appStartGameInfo struct {
	GameID string `json:"game_id"`
}

type websocketInfo struct {
	AuthBody string   `json:"auth_body"`
	WSSLink  []string `json:"wss_link"`
}

type anchorInfo struct {
	RoomID   int    `json:"room_id"`
	Username string `json:"uname"`
	UserFace string `json:"uface"`
	UID      int    `json:"uid"`
}

package biliopen_test

import (
	"context"
	biliopen "github.com/fython/bili-open-live-go"
	"go.uber.org/zap"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	projectID, err := strconv.ParseInt(os.Getenv("LIVE_PROJECT_ID"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	client := &biliopen.LiveClient{
		AppKey:    os.Getenv("LIVE_APP_KEY"),
		AppSecret: os.Getenv("LIVE_APP_SECRET"),
		ProjectID: projectID,
	}
	client.OnDanmaku = func(dm biliopen.Danmaku) {
		logger.Sugar().Infof("收到弹幕：%+v", dm)
	}

	ctx := context.Background()
	if err := client.Connect(ctx, os.Getenv("LIVE_CODE")); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second * 120)
}

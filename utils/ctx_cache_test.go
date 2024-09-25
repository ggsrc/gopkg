package utils_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ggsrc/gopkg/utils"
)

// GetBaidu wrap http invoke
func GetBaidu(ctx context.Context, t *testing.T) (string, error) {
	baiduBody, err := utils.LoadFromCtxCache(ctx, "GetBaidu", func(ctx context.Context) (string, error) {
		resp, err := http.Get("https://app.requestly.io/delay/4000/https://baidu.com")
		defer resp.Body.Close() // nolint:govet
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		bodyBytes, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		return string(bodyBytes), nil
	})
	return baiduBody, err
}

func TestLoadFromCtxCache(t *testing.T) {
	ctx := context.TODO()
	rpcCtx := utils.WithCallCache(ctx)
	startTime := time.Now()
	baiduBody, err := GetBaidu(rpcCtx, t)
	assert.NoError(t, err)
	assert.True(t, time.Since(startTime) > 4*time.Second)
	t.Log("first invoke time:", time.Since(startTime))
	startTime = time.Now()
	baiduBody2, err := GetBaidu(rpcCtx, t)
	assert.NoError(t, err)
	assert.Equal(t, baiduBody, baiduBody2)
	assert.True(t, time.Since(startTime) < 1*time.Second)
	t.Log("second invoke time:", time.Since(startTime))
	t.Log("use rpc cache")
}

func TestTryLoadFromCtxCache(t *testing.T) {
	ctx := context.TODO()
	rpcCtx := utils.WithCallCache(ctx)
	baiduBody, err := GetBaidu(rpcCtx, t)
	assert.NoError(t, err)
	exists, baiduBody2, err := utils.TryLoadFromCtxCache(rpcCtx, "GetBaidu")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, baiduBody, baiduBody2)
}

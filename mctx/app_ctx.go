package mctx

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
)

type key string

const appCtxKey = key("appCtxKey")

type AppContext struct {
	User         User             `json:"user"`
	CommonParams *ReqCommonParams `json:"common_params"`
}

// ReqCommonParams  请求通用参数
type ReqCommonParams struct {
	AppID   int32  `json:"aid"`
	AppName string `json:"app_name"`

	DeviceID         string        `json:"device_id"`
	InstallID        int64         `json:"iid"`
	Channel          string        `json:"channel"`
	DevicePlatform   string        `json:"device_platform" query:"device_platform"`
	DeviceType       string        `json:"device_type" query:"device_type"`
	DeviceBrand      string        `json:"device_brand" query:"device_brand"`
	AC               string        `json:"ac" query:"ac"`
	OSAPI            int32         `json:"os_api" query:"os_api"`
	OSVersion        string        `json:"os_version" query:"os_version"`
	VersionCode      string        `json:"version_code" query:"version_code"`
	VersionName      string        `json:"version_name" query:"version_name"`
	Language         string        `json:"language" query:"language"`
	Resolution       string        `json:"resolution" query:"resolution"`
	TimeZoneName     string        `json:"tz_name" query:"tz_name"` // 时区
	IP               string        `json:"ip" `
	RemoteIP         string        `json:"remote_ip"`
	FP               string        `json:"fp"`
	UtmSource        string        `json:"utm_source"`
	UtmMedium        string        `json:"utm_medium"`
	UtmCampaign      string        `json:"utm_campaign"`
	Idfa             string        `json:"idfa"`
	Forwarded        string        `json:"forwarded"`
	AppRegion        string        `json:"app_region"`
	SysRegion        string        `json:"sys_region"`
	AppLanguage      string        `json:"app_language"`
	SysLanguage      string        `json:"sys_language"`
	AppVersion       string        `json:"app_version"`
	ReqTime          int64         `json:"req_time"`           // unix时间戳
	Location         *LocationInfo `json:"location"`           // 位置信息
	FirstInstallTime int64         `json:"first_install_time"` // device 设备首次安装时间
	EnterFrom        string        `json:"enter_from"`
	BuildVersion     string        `json:"build_version"` // 前端代码版本

	Now time.Time `json:"now"`
}

func AppCtxFromContext(ctx context.Context) (*AppContext, bool) {
	appCtx, ok := ctx.Value(appCtxKey).(*AppContext)
	return appCtx, ok
}

func ContextWithAppCtx(ctx context.Context, value *AppContext) context.Context {
	return context.WithValue(ctx, appCtxKey, value)
}

func StringToAppCtx(s string) (*AppContext, error) {
	var appCtx AppContext
	err := sonic.UnmarshalString(s, &appCtx)
	if err != nil {
		return nil, err
	}
	if appCtx.CommonParams.Now.Unix() == 0 {
		appCtx.CommonParams.Now = time.Now()
	}
	return &appCtx, nil
}

package mctx

import (
	"fmt"
)

func NewUser(appID int32, userID string, deviceID string) User {
	return User{
		AppID:    appID,
		UserID:   userID,
		DeviceID: deviceID,
	}
}

type User struct {
	AppID    int32  `json:"app_id"`
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`

	Address     string `json:"address"`
	SignAt      string `json:"sign_at"`
	Signature   string `json:"signature"`
	AccessToken string `json:"access_token"`

	JwtString string `json:"-"`
}

func (u User) String() string {
	return fmt.Sprintf("app%d/u%s/d%s", u.AppID, u.UserID, u.DeviceID)
}

func (u User) GetAppID() int32 {
	return u.AppID
}

// IsLogin 是否登录用户
func (u User) IsLogin() bool {
	return u.UserID != ""
}

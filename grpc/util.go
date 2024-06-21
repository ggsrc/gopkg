package grpc

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/grpc-ecosystem/go-grpc-middleware/util/metautils"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/rs/zerolog"

	"github.com/ggsrc/gopkg/interceptor/metadata"
)

var (
	// CReg credID symbol regexp to find C_123
	CReg *regexp.Regexp = regexp.MustCompile(`C_\d+`)
	// NReg number regexp to find number inside credential id
	NReg *regexp.Regexp = regexp.MustCompile(`\d+`)
)

// DeviceType ...
type DeviceType = string

// Device type
const (
	UNKNOWN       DeviceType = ""
	MOBILEANDROID DeviceType = "Mobile_android"
	MOBILEIPHONE  DeviceType = "Mobile_iphone"
	MOBILEOTHER   DeviceType = "Mobile_other"
	TABLETIPAD    DeviceType = "Tablet_ipad"
	TABLETOTHER   DeviceType = "Tablet_other"
	WEB           DeviceType = "Web"
	TV            DeviceType = "TV"
)

// GetDeviceType takes http request and returns a device type
func GetDeviceType(userAgentHeader string) DeviceType {
	if isUserAgent(userAgentHeader, "Android") {
		return MOBILEANDROID
	}
	if isUserAgent(userAgentHeader, "iPhone") {
		return MOBILEIPHONE
	}
	if isUserAgent(userAgentHeader, "webOS", "BlackBerry", "Windows Phone") {
		return MOBILEOTHER
	}
	if isUserAgent(userAgentHeader, "iPad", "iPod") {
		return TABLETIPAD
	}
	if isUserAgent(userAgentHeader, "tablet", "RX-34", "FOLIO") ||
		(isUserAgent(userAgentHeader, "Kindle", "Mac OS") && isUserAgent(userAgentHeader, "Silk")) ||
		(isUserAgent(userAgentHeader, "AppleWebKit") && isUserAgent(userAgentHeader, "Silk")) {
		return TABLETOTHER
	}
	if isUserAgent(userAgentHeader, "TV", "NetCast", "boxee", "Kylo", "Roku", "DLNADOC") {
		return TV
	}

	return WEB
}

func isUserAgent(userAgent string, userAgents ...string) bool {
	for _, v := range userAgents {
		if strings.Contains(userAgent, v) {
			return true
		}
	}
	return false
}

func GetIPFromRequest(r *http.Request) (string, error) {
	var ip string
	var err error
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ", ")
		ip = ips[0]
	} else {
		ip, _, err = net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return "", err
		}
	}
	if !isIP(ip) {
		// TODO: error handling improvement
		return "", fmt.Errorf("not a valid ip: %s", ip)
	}
	return ip, nil
}

func isIP(ipstr string) (b bool) {
	ip := net.ParseIP(ipstr)
	return ip != nil
}

func GetCredIDsFromFormula(formula string) []int64 {
	credIDs := []int64{}
	var credID int64

	cRegFound := CReg.FindString(formula)
	for cRegFound != "" {
		nRegFound := NReg.FindString(cRegFound)
		credID, _ = strconv.ParseInt(nRegFound, 10, 64)
		credIDs = append(credIDs, credID)
		formula = strings.Replace(formula, cRegFound, "dummy", 1)
		// search next credID symbol
		cRegFound = CReg.FindString(formula)
	}

	return credIDs
}

func GetNumericHash(s string) uint32 {
	hash := fnv.New32a()
	hash.Write([]byte(s))

	return hash.Sum32()
}

func GetRequestSource(ctx context.Context) string {
	md := metautils.ExtractIncoming(ctx)
	return md.Get(metadata.CTX_KEY_REQUEST_SOURCE)
}

func GetJwtToken(ctx context.Context) string {
	md := metautils.ExtractIncoming(ctx)
	return md.Get(metadata.CTX_KEY_JWT_TOKEN)
}

func GetAccessToken(ctx context.Context) string {
	md := metautils.ExtractIncoming(ctx)
	return md.Get(metadata.CTX_KEY_ACCESS_TOKEN)
}

func GetGalxeId(ctx context.Context) string {
	md := metautils.ExtractIncoming(ctx)
	return md.Get(metadata.CTX_KEY_GALXE_ID)
}

func GetOrigin(ctx context.Context) string {
	md := metautils.ExtractIncoming(ctx)
	return md.Get(metadata.CTX_KEY_ORIGIN)
}

func IsRequestByApp(ctx context.Context) bool {
	return GetRequestSource(ctx) == metadata.REQUEST_SOURCE_APP
}

func IsRequestByWeb(ctx context.Context) bool {
	return GetRequestSource(ctx) == metadata.REQUEST_SOURCE_WEB
}

func IsRequestByMWeb(ctx context.Context) bool {
	return GetRequestSource(ctx) == metadata.REQUEST_SOURCE_MWEB
}

// InterceptorLogger adapts zerolog logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func InterceptorLogger(l zerolog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l := l.With().Fields(fields).Logger()
		switch lvl {
		case logging.LevelDebug:
			l.Debug().Msg(msg)
		case logging.LevelInfo:
			l.Info().Msg(msg)
		case logging.LevelWarn:
			l.Warn().Msg(msg)
		case logging.LevelError:
			l.Error().Msg(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

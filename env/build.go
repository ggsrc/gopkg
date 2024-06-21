package env

import (
	"os"
	"sync"
)

const (
	UnknownBuildTime = "BuildTime"
)

var (
	buildTime    string
	setBuildTime sync.Once
)

func init() {
	buildTime = os.Getenv("BUILD_TIME")
	if buildTime == "" {
		buildTime = os.Getenv("build_time")
	}
	if buildTime == "" {
		buildTime = os.Getenv("build-time")
	}
	if buildTime == "" {
		buildTime = os.Getenv("buildTime")
	}
	if buildTime == "" {
		buildTime = UnknownBuildTime
	}
}

func SetBuildTime(time string) {
	setBuildTime.Do(func() {
		buildTime = time
	})
}

func BuildTime() string {
	return buildTime
}

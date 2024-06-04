package env

import (
	"os"
	"sync"
)

const (
	UnknownHostName = "UnknownHost"
)

var (
	hostName    string
	setHostName sync.Once
)

func init() {
	hostName = os.Getenv("HOSTNAME")
	if hostName == "" {
		hostName = os.Getenv("hostname")
	}
	if hostName == "" {
		hostName = os.Getenv("host-name")
	}
	if hostName == "" {
		hostName = os.Getenv("OTEL_HOST_NAME")
	}
	if hostName == "" {
		hostName = UnknownHostName
	}
}

func SetHostName(name string) {
	setHostName.Do(func() {
		hostName = name
	})
}

func HostName() string {
	return hostName
}

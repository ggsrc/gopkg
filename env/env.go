package env

import "os"

func IsProduction() bool {
	return os.Getenv("ENV") == "prd"
}

func IsBeta() bool {
	return os.Getenv("ENV") == "beta"
}

func IsStaging() bool {
	return os.Getenv("ENV") == "stg"
}

func IsLocal() bool {
	return os.Getenv("ENV") == ""
}

func IsUnitTest() bool {
	return os.Getenv("ENV") == "test"
}

func Env() string {
	e := os.Getenv("ENV")
	if e == "" {
		e = os.Getenv("OTEL_DEPLOYMENT_ENVIRONMENT")
	}
	if e == "" {
		return "local"
	}
	return e
}

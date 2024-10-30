package httputil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ggsrc/gopkg/httputil"
)

func TestHttpClient(t *testing.T) {
	httpClient := httputil.NewDefaultHttpClient("test", true)
	get, err := httpClient.Get("https://graphigo.prd.galaxy.eco/")
	assert.NoError(t, err)
	assert.NotNil(t, get)
}

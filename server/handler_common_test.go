package server

import (
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"testing"
)

func ResponseData(t *testing.T, resp *http.Response) []byte {
	defer func(body io.ReadCloser) { _ = body.Close() }(resp.Body)
	d, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return d
}

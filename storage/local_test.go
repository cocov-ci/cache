//go:build unit

package storage

import (
	"github.com/cocov-ci/cache/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func makeLocalClient(t *testing.T) (string, Provider) {
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	apiURL := os.Getenv("API_URL")
	if len(apiURL) == 0 {
		apiURL = "http://localhost:3000"
	}
	apiToken := os.Getenv("API_TOKEN")
	apiClient := api.New(apiURL, apiToken)

	c, err := NewLocalStorage(tmpDir, apiClient)
	require.NoError(t, err)

	return tmpDir, c
}

func TestLocal(t *testing.T) {
	t.Run("initialize", func(t *testing.T) {
		tmpDir, _ := makeLocalClient(t)
		stat, err := os.Stat(filepath.Join(tmpDir, "tool-cache"))
		require.NoError(t, err)
		assert.True(t, stat.IsDir())

		stat, err = os.Stat(filepath.Join(tmpDir, "artifacts"))
		require.NoError(t, err)
		assert.True(t, stat.IsDir())
	})

	t.Run("set artifact", func(t *testing.T) {
		tmpDir, provider := makeLocalClient(t)
		testSetArtifact(t, provider, func(t *testing.T) int {
			return countFiles(t, tmpDir)
		})
	})

	t.Run("read artifact", func(t *testing.T) {
		_, provider := makeLocalClient(t)
		testReadArtifact(t, provider)
	})

	t.Run("read artifact (not found)", func(t *testing.T) {
		_, provider := makeLocalClient(t)
		testArtifactNotFound(t, provider)
	})

	t.Run("touch artifact", func(t *testing.T) {
		_, provider := makeLocalClient(t)
		testArtifactTouch(t, provider)
	})

	t.Run("delete artifact", func(t *testing.T) {
		tmpDir, provider := makeLocalClient(t)
		testArtifactDelete(t, provider, func(t *testing.T) int {
			return countFiles(t, tmpDir)
		})
	})

	t.Run("purge repository", func(t *testing.T) {
		tmpDir, provider := makeLocalClient(t)
		testPurgeRepository(t, provider, func(t *testing.T) int {
			return countFiles(t, tmpDir)
		})
	})

	t.Run("set tool", func(t *testing.T) {
		tmpDir, provider := makeLocalClient(t)
		testSetTool(t, provider, func(t *testing.T) int {
			return countFiles(t, tmpDir)
		})
	})

	t.Run("read tool", func(t *testing.T) {
		_, provider := makeLocalClient(t)
		testReadTool(t, provider)
	})

	t.Run("read tool (not found)", func(t *testing.T) {
		_, provider := makeLocalClient(t)
		testToolNotFound(t, provider)
	})

	t.Run("touch tool", func(t *testing.T) {
		_, provider := makeLocalClient(t)
		testToolTouch(t, provider)
	})

	t.Run("delete tool", func(t *testing.T) {
		tmpDir, provider := makeLocalClient(t)
		testToolDelete(t, provider, func(t *testing.T) int {
			return countFiles(t, tmpDir)
		})
	})

	t.Run("purge tool", func(t *testing.T) {
		tmpDir, provider := makeLocalClient(t)
		testToolPurge(t, provider, func(t *testing.T) int {
			return countFiles(t, tmpDir)
		})
	})
}

package storage

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	fileContents := "this is a test"
	fileLocator := ArtifactDescriptor("cache", "foobar")

	var client Provider

	// Should create required directories
	t.Run("initialize", func(t *testing.T) {
		client, err = NewLocalStorage(tmpDir)
		require.NoError(t, err)

		stat, err := os.Stat(filepath.Join(tmpDir, "tool-cache"))
		require.NoError(t, err)
		assert.True(t, stat.IsDir())

		stat, err = os.Stat(filepath.Join(tmpDir, "artifacts"))
		require.NoError(t, err)
		assert.True(t, stat.IsDir())
	})

	t.Run("set artifact", func(t *testing.T) {
		data := []byte(fileContents)
		dataReader := bytes.NewReader(data)
		readerCloser := io.NopCloser(dataReader)

		err = client.Set(fileLocator, "text/plain", len(data), readerCloser)
		require.NoError(t, err)
	})

	t.Run("read artifact", func(t *testing.T) {
		meta, reader, err := client.Get(fileLocator)
		require.NoError(t, err)
		assert.Equal(t, len(fileContents), meta.Size)
		assert.Equal(t, "text/plain", meta.Mime)
		assert.InDelta(t, time.Now().UTC().Unix(), meta.CreatedAt.Unix(), 10)
		assert.InDelta(t, time.Now().UTC().Unix(), meta.AccessedAt.Unix(), 10)

		data, err := io.ReadAll(reader)
		_ = reader.Close()
		require.NoError(t, err)

		assert.Equal(t, fileContents, string(data))
	})

	t.Run("read artifact (not found)", func(t *testing.T) {
		meta, reader, err := client.Get(ArtifactDescriptor("cache", "foobars"))
		assert.Nil(t, meta)
		assert.Nil(t, reader)
		assert.ErrorAs(t, err, &ErrNotExist{})
	})

	t.Run("touch", func(t *testing.T) {
		currentMeta, err := client.MetadataOf(fileLocator)
		require.NoError(t, err)

		err = client.Touch(fileLocator)
		require.NoError(t, err)

		newMeta, err := client.MetadataOf(fileLocator)
		require.NoError(t, err)

		assert.Equal(t, currentMeta.CreatedAt, newMeta.CreatedAt)
		assert.NotEqual(t, currentMeta.AccessedAt, newMeta.AccessedAt)
	})

	t.Run("total size", func(t *testing.T) {
		size, err := client.TotalSize(KindArtifact)
		require.NoError(t, err)
		assert.Equal(t, int64(len(fileContents)), size)
	})

	t.Run("delete", func(t *testing.T) {
		err = client.Delete(fileLocator)
		assert.NoError(t, err)

		read, err := os.ReadDir(filepath.Join(tmpDir, "artifacts", fileLocator.groupPath()))
		assert.NoError(t, err)
		assert.Empty(t, read)
	})

	t.Run("delete group", func(t *testing.T) {
		err = client.DeleteGroup(ArtifactParentDescriptor("cache"))
		assert.NoError(t, err)

		read, err := os.ReadDir(filepath.Join(tmpDir, "artifacts"))
		assert.NoError(t, err)
		assert.Empty(t, read)
	})
}

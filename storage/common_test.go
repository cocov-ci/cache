package storage

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"github.com/cocov-ci/cache/locator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"io/fs"
	"math/rand"
	"path/filepath"
	"testing"
	"time"
)

func randInt(start, stop int) int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return r.Intn(stop-start) + start
}

func randHex() string {
	buf := make([]byte, randInt(3, 10))
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Read(buf)
	return hex.EncodeToString(buf)
}

func pushArtifact(t *testing.T, provider Provider) (*locator.ArtifactLocator, string) {
	contents := randHex()
	data := []byte(contents)
	dataReader := bytes.NewReader(data)
	readerCloser := io.NopCloser(dataReader)

	name := randHex()
	sha := sha1.New()
	sha.Write([]byte(name))
	hash := hex.EncodeToString(sha.Sum(nil))

	loc := &locator.ArtifactLocator{
		RepositoryID:   1,
		NameHash:       hash,
		Name:           name,
		RepositoryName: "cache",
	}

	err := provider.SetArtifact(loc, "text/plain", int64(len(data)), readerCloser)
	require.NoError(t, err)

	return loc, contents
}

func pushTool(t *testing.T, provider Provider) (*locator.ToolLocator, string) {
	contents := randHex()
	data := []byte(contents)
	dataReader := bytes.NewReader(data)
	readerCloser := io.NopCloser(dataReader)

	name := randHex()
	sha := sha1.New()
	sha.Write([]byte(name))
	hash := hex.EncodeToString(sha.Sum(nil))

	loc := &locator.ToolLocator{
		NameHash: hash,
		Name:     name,
	}

	err := provider.SetTool(loc, "text/plain", int64(len(data)), readerCloser)
	require.NoError(t, err)

	return loc, contents
}

func countFiles(t *testing.T, path string) int {
	currentFileCount := 0
	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			currentFileCount += 1
		}
		return nil
	})
	require.NoError(t, err)
	return currentFileCount
}

// Artifact

func testSetArtifact(t *testing.T, p Provider, countFSItems func(t *testing.T) int) {
	countBefore := countFSItems(t)
	loc, contents := pushArtifact(t, p)
	countAfter := countFSItems(t)
	assert.Greater(t, countAfter, countBefore)

	// read it
	meta, reader, err := p.GetArtifact(loc)
	require.NoError(t, err)
	assert.Equal(t, loc.NameHash, meta.NameHash)
	assert.Equal(t, loc.Name, meta.Name)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, contents, string(data))
}

func testReadArtifact(t *testing.T, p Provider) {
	loc, contents := pushArtifact(t, p)

	meta, reader, err := p.GetArtifact(loc)
	require.NoError(t, err)
	assert.Equal(t, int64(len(contents)), meta.Size)
	assert.Equal(t, "text/plain", meta.Mime)

	data, err := io.ReadAll(reader)
	_ = reader.Close()
	require.NoError(t, err)

	assert.Equal(t, contents, string(data))
}

func testArtifactNotFound(t *testing.T, p Provider) {
	meta, reader, err := p.GetArtifact(&locator.ArtifactLocator{
		RepositoryID:   0,
		NameHash:       "test",
		Name:           "test",
		RepositoryName: "foobar",
	})
	assert.Nil(t, meta)
	assert.Nil(t, reader)
	assert.ErrorAs(t, err, &ErrNotExist{})
}

func testArtifactTouch(t *testing.T, p Provider) {
	loc, _ := pushArtifact(t, p)
	currentMeta, err := p.GetArtifactMeta(loc)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	err = p.TouchArtifact(loc)
	require.NoError(t, err)

	newMeta, err := p.GetArtifactMeta(loc)
	require.NoError(t, err)

	assert.Equal(t, currentMeta.CreatedAt, newMeta.CreatedAt)
	assert.NotEqual(t, currentMeta.LastUsedAt, newMeta.LastUsedAt)
}

func testArtifactDelete(t *testing.T, p Provider, fsCounter func(t *testing.T) int) {
	loc, _ := pushArtifact(t, p)

	beforeCount := fsCounter(t)
	err := p.DeleteArtifact(loc)
	require.NoError(t, err)
	afterCount := fsCounter(t)
	assert.Less(t, afterCount, beforeCount)
}

func testPurgeRepository(t *testing.T, p Provider, fsCounter func(t *testing.T) int) {
	loc, _ := pushArtifact(t, p)

	beforeCount := fsCounter(t)
	err := p.PurgeRepository(loc.RepositoryID)
	require.NoError(t, err)
	afterCount := fsCounter(t)
	assert.Less(t, afterCount, beforeCount)
	assert.Zero(t, afterCount)
}

// Tool

func testSetTool(t *testing.T, p Provider, countFSItems func(t *testing.T) int) {
	countBefore := countFSItems(t)
	loc, contents := pushTool(t, p)
	countAfter := countFSItems(t)
	assert.Greater(t, countAfter, countBefore)

	// read it
	meta, reader, err := p.GetTool(loc)
	require.NoError(t, err)
	assert.Equal(t, loc.NameHash, meta.NameHash)
	assert.Equal(t, loc.Name, meta.Name)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, contents, string(data))
}

func testReadTool(t *testing.T, p Provider) {
	loc, contents := pushTool(t, p)

	meta, reader, err := p.GetTool(loc)
	require.NoError(t, err)
	assert.Equal(t, int64(len(contents)), meta.Size)
	assert.Equal(t, "text/plain", meta.Mime)

	data, err := io.ReadAll(reader)
	_ = reader.Close()
	require.NoError(t, err)

	assert.Equal(t, contents, string(data))
}

func testToolNotFound(t *testing.T, p Provider) {
	meta, reader, err := p.GetTool(&locator.ToolLocator{
		NameHash: "test",
		Name:     "test",
	})
	assert.Nil(t, meta)
	assert.Nil(t, reader)
	assert.ErrorAs(t, err, &ErrNotExist{})
}

func testToolTouch(t *testing.T, p Provider) {
	loc, _ := pushTool(t, p)
	currentMeta, err := p.GetToolMeta(loc)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	err = p.TouchTool(loc)
	require.NoError(t, err)

	newMeta, err := p.GetToolMeta(loc)
	require.NoError(t, err)

	assert.Equal(t, currentMeta.CreatedAt, newMeta.CreatedAt)
	assert.NotEqual(t, currentMeta.LastUsedAt, newMeta.LastUsedAt)
}

func testToolDelete(t *testing.T, p Provider, fsCounter func(t *testing.T) int) {
	loc, _ := pushTool(t, p)

	beforeCount := fsCounter(t)
	err := p.DeleteTool(loc)
	require.NoError(t, err)
	afterCount := fsCounter(t)
	assert.Less(t, afterCount, beforeCount)
}

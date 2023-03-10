package storage

import (
	"crypto/sha1"
	"encoding/base32"
	"strings"
	"time"
)

type Kind int

const (
	KindTool Kind = iota
	KindArtifact
)

type Item struct {
	CreatedAt  time.Time `json:"created_at" tag:"app.cocov.cache.item.created_at"`
	AccessedAt time.Time `json:"accessed_at" tag:"app.cocov.cache.item.accessed_at"`
	Size       int       `json:"size,omitempty" tag:"app.cocov.cache.item.size"`
	Mime       string    `json:"mime,omitempty" tag:"app.cocov.cache.item.mime"`
}

type ObjectDescriptor interface {
	// PathComponents returns a list of path components making the path to the
	// item being represented by a locator instance.
	PathComponents() []string

	// Indicates whether an item may have siblings. Groupable elements may have
	// their first branch removed along with all its siblings, whilst the
	// opposite cannot.
	groupable() bool

	groupPath() string

	kind() Kind
}

func ArtifactDescriptor(repositoryName string, itemKey string) ObjectDescriptor {
	return artifactDescriptor{parent: repositoryName, key: itemKey}
}

func ArtifactParentDescriptor(repositoryName string) ObjectDescriptor {
	return artifactDescriptor{parent: repositoryName}
}

func ToolDescriptor(itemKey string) ObjectDescriptor {
	return toolDescriptor{key: itemKey}
}

type artifactDescriptor struct {
	parent string
	key    string
}

func (a artifactDescriptor) normalizeParent() string {
	normalized := strings.ToLower(a.parent)
	sha := sha1.New()
	sha.Write([]byte(normalized))
	return strings.ToLower(base32.StdEncoding.EncodeToString(sha.Sum(nil)))
}

func (a artifactDescriptor) PathComponents() []string {
	norm := a.normalizeParent()
	if len(a.key) == 0 {
		return []string{norm}
	}
	return []string{norm, a.key}
}

func (a artifactDescriptor) groupPath() string { return a.normalizeParent() }

func (artifactDescriptor) groupable() bool { return true }

func (artifactDescriptor) kind() Kind { return KindArtifact }

type toolDescriptor struct {
	key string
}

func (t toolDescriptor) groupPath() string { panic("toolDescriptor is not groupable") }

func (t toolDescriptor) PathComponents() []string { return []string{t.key} }

func (toolDescriptor) groupable() bool { return false }

func (toolDescriptor) kind() Kind { return KindTool }

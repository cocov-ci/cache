package storage

import (
	"github.com/cocov-ci/cache/api"
	"github.com/cocov-ci/cache/locator"
	"io"
)

type Provider interface {
	GetArtifactMeta(locator *locator.ArtifactLocator) (*api.GetArtifactMetaOutput, error)
	GetArtifact(locator *locator.ArtifactLocator) (*api.GetArtifactMetaOutput, io.ReadCloser, error)
	SetArtifact(locator *locator.ArtifactLocator, mime string, objectSize int64, stream io.ReadCloser) error
	TouchArtifact(locator *locator.ArtifactLocator) error
	DeleteArtifact(locator *locator.ArtifactLocator) error
	PurgeRepository(id int64) error

	GetToolMeta(locator *locator.ToolLocator) (*api.GetToolMetaOutput, error)
	GetTool(locator *locator.ToolLocator) (*api.GetToolMetaOutput, io.ReadCloser, error)
	SetTool(locator *locator.ToolLocator, mime string, objectSize int64, stream io.ReadCloser) error
	TouchTool(locator *locator.ToolLocator) error
	DeleteTool(locator *locator.ToolLocator) error
	PurgeTool() error
}

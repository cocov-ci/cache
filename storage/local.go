package storage

import (
	"fmt"
	"github.com/cocov-ci/cache/api"
	"github.com/cocov-ci/cache/locator"
	"go.uber.org/zap"
	"io"
	"os"
	"path/filepath"
)

func ensureDir(at string) error {
	retrying := false
	for {
		stat, err := os.Stat(at)
		if os.IsNotExist(err) && !retrying {
			if err = os.MkdirAll(at, 0755); err != nil {
				return fmt.Errorf("mkdir_p %s: %w", at, err)
			}
			retrying = true
			continue
		} else if err != nil {
			return nil
		}

		if stat.IsDir() {
			return nil
		}

		return fmt.Errorf("%s: exists and is not a directory", at)
	}
}

func NewLocalStorage(basePath string, client api.Client) (Provider, error) {
	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}

	toolPath := filepath.Join(basePath, "tool-cache")
	artifactPath := filepath.Join(basePath, "artifacts")

	paths := []string{basePath, toolPath, artifactPath}
	for _, p := range paths {
		if err = ensureDir(p); err != nil {
			return nil, err
		}
	}

	return LocalStorage{
		log:          zap.L().With(zap.String("component", "LocalStorage")),
		basePath:     basePath,
		toolPath:     toolPath,
		artifactPath: artifactPath,
		api:          client,
	}, nil
}

type LocalStorage struct {
	basePath     string
	toolPath     string
	artifactPath string
	api          api.Client
	log          *zap.Logger
}

func (l LocalStorage) GetArtifactMeta(locator *locator.ArtifactLocator) (*api.GetArtifactMetaOutput, error) {
	return l.api.GetArtifactMeta(api.GetArtifactMetaInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "local",
	})
}

func (l LocalStorage) groupPath(repoID int64) (string, error) {
	path := filepath.Join(l.artifactPath, fmt.Sprintf("%d", repoID))
	err := os.MkdirAll(path, 0755)
	return path, err
}

func (l LocalStorage) makeToolPath(toolID int64) string {
	return filepath.Join(l.toolPath, fmt.Sprintf("%d", toolID))
}

func (l LocalStorage) makeArtifactPath(id, repoID int64) (string, error) {
	groupPath, err := l.groupPath(repoID)
	return filepath.Join(groupPath, fmt.Sprintf("%d", id)), err
}

func (l LocalStorage) GetArtifact(locator *locator.ArtifactLocator) (*api.GetArtifactMetaOutput, io.ReadCloser, error) {
	meta, err := l.api.GetArtifactMeta(api.GetArtifactMetaInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "local",
	})
	if api.IsNotFound(err) {
		return nil, nil, ErrNotExist{}
	} else if err != nil {
		return nil, nil, err
	}

	objPath, err := l.makeArtifactPath(meta.ID, locator.RepositoryID)
	if err != nil {
		return nil, nil, err
	}

	stream, err := os.Open(objPath)
	if os.IsNotExist(err) {
		return nil, nil, ErrNotExist{}
	} else if err != nil {
		return nil, nil, err
	}

	return meta, stream, nil
}

func (l LocalStorage) SetArtifact(locator *locator.ArtifactLocator, mime string, objectSize int64, stream io.ReadCloser) error {
	defer func() { _ = stream.Close() }()

	meta, err := l.api.RegisterArtifact(api.RegisterArtifactInput{
		RepositoryID: locator.RepositoryID,
		Name:         locator.Name,
		NameHash:     locator.NameHash,
		Size:         objectSize,
		Mime:         mime,
		Engine:       "local",
	})
	if err != nil {

		return err
	}

	deleteArgs := api.DeleteArtifactInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "local",
	}

	objPath, err := l.makeArtifactPath(meta.ID, locator.RepositoryID)
	if err != nil {
		l.log.Error("Failed creating artifact path", zap.Error(err), zap.Int64("repositoryID", locator.RepositoryID), zap.Int64("itemID", meta.ID))
		if delErr := l.api.DeleteArtifact(deleteArgs); delErr != nil {
			l.log.Error("Failed removing item from API", zap.Error(err))
		}
		return err
	}

	f, err := os.OpenFile(objPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		l.log.Error("Failed opening item path for write", zap.Error(err), zap.Int64("repositoryID", locator.RepositoryID), zap.Int64("itemID", meta.ID), zap.String("path", objPath))
		if delErr := l.api.DeleteArtifact(deleteArgs); delErr != nil {
			l.log.Error("Failed removing item from API", zap.Error(err))
		}
		return err
	}

	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, stream)
	if err != nil {
		l.log.Error("Failed streaming data into local fs", zap.Error(err))
		if delErr := l.api.DeleteArtifact(deleteArgs); delErr != nil {
			l.log.Error("Failed removing item from API", zap.Error(err))
		}
		return err
	}

	if err = f.Sync(); err != nil {
		l.log.Error("Error syncing file descriptor", zap.Error(err))
		if delErr := l.api.DeleteArtifact(deleteArgs); delErr != nil {
			l.log.Error("Failed removing item from API", zap.Error(err))
		}
		return err
	}

	return nil
}

func (l LocalStorage) TouchArtifact(locator *locator.ArtifactLocator) error {
	return l.api.TouchArtifact(api.TouchArtifactInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "local",
	})
}

func (l LocalStorage) DeleteArtifact(locator *locator.ArtifactLocator) error {
	meta, err := l.api.GetArtifactMeta(api.GetArtifactMetaInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "local",
	})
	if api.IsNotFound(err) {
		return nil
	}

	objPath, err := l.makeArtifactPath(meta.ID, locator.RepositoryID)
	if err != nil {
		l.log.Error("Failed creating artifact path", zap.Error(err), zap.Int64("repositoryID", locator.RepositoryID), zap.Int64("itemID", meta.ID))
		return err
	}

	if err = os.Remove(objPath); err != nil {
		return err
	}

	return l.api.DeleteArtifact(api.DeleteArtifactInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "local",
	})
}

func (l LocalStorage) PurgeRepository(id int64) error {
	// Worst case scenario we will have created a directory to immediately
	// delete it afterwards.
	gp, err := l.groupPath(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(gp)
}

func (l LocalStorage) GetToolMeta(locator *locator.ToolLocator) (*api.GetToolMetaOutput, error) {
	return l.api.GetToolMeta(api.GetToolMetaInput{
		NameHash: locator.NameHash,
		Engine:   "local",
	})
}

func (l LocalStorage) GetTool(locator *locator.ToolLocator) (*api.GetToolMetaOutput, io.ReadCloser, error) {
	meta, err := l.api.GetToolMeta(api.GetToolMetaInput{
		NameHash: locator.NameHash,
		Engine:   "local",
	})
	if api.IsNotFound(err) {
		return nil, nil, ErrNotExist{}
	} else if err != nil {
		return nil, nil, err
	}

	objPath := l.makeToolPath(meta.ID)
	if err != nil {
		return nil, nil, err
	}

	stream, err := os.Open(objPath)
	if os.IsNotExist(err) {
		return nil, nil, ErrNotExist{}
	} else if err != nil {
		return nil, nil, err
	}

	return meta, stream, nil
}

func (l LocalStorage) SetTool(locator *locator.ToolLocator, mime string, objectSize int64, stream io.ReadCloser) error {
	defer func() { _ = stream.Close() }()

	meta, err := l.api.RegisterTool(api.RegisterToolInput{
		Name:     locator.Name,
		NameHash: locator.NameHash,
		Size:     objectSize,
		Mime:     mime,
		Engine:   "local",
	})
	if err != nil {

		return err
	}

	deleteArgs := api.DeleteToolInput{
		NameHash: locator.NameHash,
		Engine:   "local",
	}

	objPath := l.makeToolPath(meta.ID)

	f, err := os.OpenFile(objPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		l.log.Error("Failed opening tool path for write", zap.Error(err), zap.Int64("toolID", meta.ID), zap.String("path", objPath))
		if delErr := l.api.DeleteTool(deleteArgs); delErr != nil {
			l.log.Error("Failed removing tool from API", zap.Error(err))
		}
		return err
	}

	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, stream)
	if err != nil {
		l.log.Error("Failed streaming data into local fs", zap.Error(err))
		if delErr := l.api.DeleteTool(deleteArgs); delErr != nil {
			l.log.Error("Failed removing tool from API", zap.Error(err))
		}
		return err
	}

	if err = f.Sync(); err != nil {
		l.log.Error("Error syncing file descriptor", zap.Error(err))
		if delErr := l.api.DeleteTool(deleteArgs); delErr != nil {
			l.log.Error("Failed removing tool from API", zap.Error(err))
		}
		return err
	}

	return nil
}

func (l LocalStorage) TouchTool(locator *locator.ToolLocator) error {
	return l.api.TouchTool(api.TouchToolInput{
		NameHash: locator.NameHash,
		Engine:   "local",
	})
}

func (l LocalStorage) DeleteTool(locator *locator.ToolLocator) error {
	meta, err := l.api.GetToolMeta(api.GetToolMetaInput{
		NameHash: locator.NameHash,
		Engine:   "local",
	})
	if api.IsNotFound(err) {
		return nil
	}

	objPath := l.makeToolPath(meta.ID)

	if err = os.Remove(objPath); err != nil {
		return err
	}

	return l.api.DeleteTool(api.DeleteToolInput{
		NameHash: locator.NameHash,
		Engine:   "local",
	})
}

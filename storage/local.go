package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
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

func NewLocalStorage(basePath string) (Provider, error) {
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
		basePath:     basePath,
		toolPath:     toolPath,
		artifactPath: artifactPath,
	}, nil
}

type LocalStorage struct {
	basePath     string
	toolPath     string
	artifactPath string
}

func (l LocalStorage) pathOf(locator ObjectDescriptor) string {
	components := make([]string, 1, 3)
	if locator.kind() == KindArtifact {
		components[0] = l.artifactPath
	} else {
		components[0] = l.toolPath
	}
	components = append(components, locator.PathComponents()...)

	return filepath.Join(components...)
}

func (l LocalStorage) basePathOf(k Kind) string {
	if k == KindArtifact {
		return l.artifactPath
	}

	return l.toolPath
}

func (l LocalStorage) groupPath(locator ObjectDescriptor) string {
	components := make([]string, 1, 2)
	components[0] = l.basePathOf(locator.kind())

	components = append(components, locator.groupPath())
	return filepath.Join(components...)
}

func burninate(path string) {
	_ = os.RemoveAll(path + ".meta")
	_ = os.RemoveAll(path)
}

func ensureConsistencyOf(path string) (*Item, error) {
	metaFilePath := path + ".meta"
	metaStat, err := os.Stat(metaFilePath)
	if os.IsNotExist(err) {
		_ = os.Remove(path)
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	if metaStat.IsDir() {
		burninate(path)
		return nil, nil
	}

	var meta Item
	f, err := os.Open(metaFilePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if err = json.NewDecoder(f).Decode(&meta); err != nil {
		burninate(path)
		return nil, nil
	}

	return &meta, nil
}

func writeMeta(target string, meta *Item) error {
	m, err := os.OpenFile(target+".meta", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0655)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()

	if err = json.NewEncoder(m).Encode(meta); err != nil {
		return err
	}

	if err = m.Sync(); err != nil {
		return err
	}

	return nil
}

func (l LocalStorage) MetadataOf(locator ObjectDescriptor) (*Item, error) {
	p := l.pathOf(locator)
	meta, err := ensureConsistencyOf(p)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, ErrNotExist{path: p}
	}

	return meta, nil
}

func (l LocalStorage) Get(locator ObjectDescriptor) (*Item, io.ReadCloser, error) {
	meta, err := l.MetadataOf(locator)
	if err != nil {
		return nil, nil, err
	}

	f, err := os.Open(l.pathOf(locator))
	if err != nil {
		return nil, nil, err
	}

	return meta, f, nil
}

func (l LocalStorage) Set(locator ObjectDescriptor, mime string, objectSize int, stream io.ReadCloser) error {
	meta := Item{
		CreatedAt:  time.Now().UTC(),
		AccessedAt: time.Now().UTC(),
		Size:       objectSize,
		Mime:       mime,
	}
	target := l.pathOf(locator)

	err := ensureDir(filepath.Dir(target))
	if err != nil {
		return err
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0655)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, stream)
	if err != nil {
		burninate(target)
		return err
	}

	if err = f.Sync(); err != nil {
		burninate(target)
		return err
	}

	if err = writeMeta(target, &meta); err != nil {
		burninate(target)
		return err
	}

	return nil
}

func (l LocalStorage) Touch(locator ObjectDescriptor) error {
	p := l.pathOf(locator)
	m, err := l.MetadataOf(locator)
	if err != nil {
		return err
	}
	m.AccessedAt = time.Now().UTC()
	return writeMeta(p, m)
}

func (l LocalStorage) Delete(locator ObjectDescriptor) error {
	p := l.pathOf(locator)
	_, err := l.MetadataOf(locator)
	if err != nil {
		return err
	}
	burninate(p)
	return nil
}

func (l LocalStorage) DeleteGroup(locator ObjectDescriptor) error {
	p := l.pathOf(locator)
	if !locator.groupable() {
		return ErrNotGroupable{path: p}
	}

	return os.RemoveAll(l.groupPath(locator))
}

func (l LocalStorage) TotalSize(kind Kind) (int64, error) {
	startPath := l.basePathOf(kind)
	var size int64 = 0

	ch := make(chan string, 10)
	processorDone := make(chan error)
	go func() {
		for p := range ch {
			f, err := os.Open(p)
			if err != nil {
				continue
			}
			var meta Item
			if err = json.NewDecoder(f).Decode(&meta); err != nil {
				_ = f.Close()
				continue
			}
			_ = f.Close()
			size += int64(meta.Size)
		}
		close(processorDone)
	}()

	err := filepath.WalkDir(startPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".meta") {
			ch <- path
		}
		return nil
	})
	close(ch)
	if err != nil {
		return 0, err
	}

	<-processorDone
	return size, nil
}

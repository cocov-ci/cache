package locator

import (
	"fmt"
	"github.com/cocov-ci/cache/logging"
	"github.com/cocov-ci/cache/redis"
	"github.com/go-chi/chi/v5"
	"net/http"
)

type HTTPError struct {
	Message string
	Status  int
}

func (h HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", h.Status, h.Message)
}

type Locator interface {
	LockKey() string
	LoadFrom(redis redis.Client, r *http.Request) error
}

type ArtifactLocator struct {
	RepositoryID   int64
	NameHash       string
	Name           string
	RepositoryName string
}

func (a *ArtifactLocator) LockKey() string {
	return fmt.Sprintf("artifact:%d:%s", a.RepositoryID, a.NameHash)
}
func (a *ArtifactLocator) LoadFrom(redis redis.Client, r *http.Request) error {
	log := logging.GetLogger(r)

	itemKey := chi.URLParam(r, "id")
	if itemKey == "" {
		log.Error("Rejecting request lacking id")
		return HTTPError{
			Message: "Missing ID",
			Status:  http.StatusBadRequest,
		}
	}

	repoID, err := redis.RepoDataFromJID(r.Header.Get("Cocov-Job-ID"))
	if err != nil {
		return err
	}

	if repoID == nil {
		log.Error("Rejecting request without client authorization")
		return HTTPError{Status: http.StatusForbidden}
	}

	a.RepositoryID = repoID.ID
	a.NameHash = itemKey
	a.Name = r.Header.Get("Filename")
	a.RepositoryName = repoID.Name

	return nil
}

type ToolLocator struct {
	NameHash string
	Name     string
}

func (t *ToolLocator) LockKey() string {
	return fmt.Sprintf("tool:%s", t.NameHash)
}
func (t *ToolLocator) LoadFrom(redis redis.Client, r *http.Request) error {
	log := logging.GetLogger(r)

	itemKey := chi.URLParam(r, "id")
	if itemKey == "" {
		log.Error("Rejecting request lacking id")
		return HTTPError{
			Message: "Missing ID",
			Status:  http.StatusBadRequest,
		}
	}

	repoID, err := redis.RepoDataFromJID(r.Header.Get("Cocov-Job-ID"))
	if err != nil {
		return err
	}

	if repoID == nil {
		log.Error("Rejecting request without client authorization")
		return HTTPError{Status: http.StatusForbidden}
	}

	t.NameHash = itemKey
	t.Name = r.Header.Get("Filename")
	return nil
}

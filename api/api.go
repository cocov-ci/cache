package api

import (
	"fmt"
	"github.com/levigross/grequests"
	"strings"
)

type HTTPNotFound struct{}

func (HTTPNotFound) Error() string { return "Not found" }

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(HTTPNotFound)
	return ok
}

type Client interface {
	RegisterArtifact(RegisterArtifactInput) (*RegisterArtifactOutput, error)
	RegisterTool(RegisterToolInput) (*RegisterToolOutput, error)
	GetArtifactMeta(GetArtifactMetaInput) (*GetArtifactMetaOutput, error)
	GetToolMeta(GetToolMetaInput) (*GetToolMetaOutput, error)
	DeleteArtifact(DeleteArtifactInput) error
	DeleteTool(input DeleteToolInput) error
	TouchArtifact(TouchArtifactInput) error
	TouchTool(input TouchToolInput) error
	Ping() error
}

func New(baseURL string, token string) Client {
	baseURL = strings.TrimSuffix(baseURL, "/")
	return impl{baseURL: baseURL, token: token}
}

type impl struct {
	baseURL string
	token   string
}

func (i impl) repoURL(repositoryID int64, format string, a ...any) string {
	return fmt.Sprintf("%s/v1/repositories/%d/cache/%s", i.baseURL, repositoryID, fmt.Sprintf(format, a...))
}

func (i impl) toolURL(format string, a ...any) string {
	return fmt.Sprintf("%s/v1/cache/tools/%s", i.baseURL, fmt.Sprintf(format, a...))
}

func processRequest[T any](i impl, method, url string, params map[string]string) (*T, error) {
	r, err := grequests.DoRegularRequest(method, url, &grequests.RequestOptions{
		Params: params,
		Headers: map[string]string{
			"Accept":        "application/json",
			"Authorization": "bearer " + i.token,
		},
	})
	if err != nil {
		return nil, err
	}

	if r.StatusCode == 404 {
		return nil, HTTPNotFound{}
	}

	if !r.Ok {
		return nil, fmt.Errorf("request failed with status %d: %s", r.StatusCode, r.String())
	}

	var into T
	if err = r.JSON(&into); err != nil {
		return nil, err
	}

	return &into, nil
}

func processRequestNoBody(i impl, method, url string, params map[string]string) error {
	r, err := grequests.DoRegularRequest(method, url, &grequests.RequestOptions{
		Params: params,
		Headers: map[string]string{
			"Accept":        "application/json",
			"Authorization": "bearer " + i.token,
		},
	})
	if err != nil {
		return err
	}

	if r.StatusCode == 404 {
		return HTTPNotFound{}
	}

	if !r.Ok {
		return fmt.Errorf("request failed with status %d: %s", r.StatusCode, r.String())
	}

	return nil
}

func (i impl) RegisterArtifact(input RegisterArtifactInput) (*RegisterArtifactOutput, error) {
	return processRequest[RegisterArtifactOutput](i, "POST", i.repoURL(input.RepositoryID, ""), map[string]string{
		"name":      input.Name,
		"name_hash": input.NameHash,
		"size":      fmt.Sprintf("%d", input.Size),
		"mime":      input.Mime,
		"engine":    input.Engine,
	})
}

func (i impl) GetArtifactMeta(input GetArtifactMetaInput) (*GetArtifactMetaOutput, error) {
	return processRequest[GetArtifactMetaOutput](i, "GET", i.repoURL(input.RepositoryID, "%s/meta", input.NameHash), map[string]string{
		"engine": input.Engine,
	})
}

func (i impl) DeleteArtifact(input DeleteArtifactInput) error {
	return processRequestNoBody(i, "DELETE", i.repoURL(input.RepositoryID, "%s", input.NameHash), map[string]string{
		"engine": input.Engine,
	})
}

func (i impl) TouchArtifact(input TouchArtifactInput) error {
	return processRequestNoBody(i, "PATCH", i.repoURL(input.RepositoryID, "%s/touch", input.NameHash), map[string]string{
		"engine": input.Engine,
	})
}

func (i impl) RegisterTool(input RegisterToolInput) (*RegisterToolOutput, error) {
	return processRequest[RegisterToolOutput](i, "POST", i.toolURL(""), map[string]string{
		"name":      input.Name,
		"name_hash": input.NameHash,
		"size":      fmt.Sprintf("%d", input.Size),
		"mime":      input.Mime,
		"engine":    input.Engine,
	})
}

func (i impl) GetToolMeta(input GetToolMetaInput) (*GetToolMetaOutput, error) {
	return processRequest[GetToolMetaOutput](i, "GET", i.toolURL("%s/meta", input.NameHash), map[string]string{
		"engine": input.Engine,
	})
}

func (i impl) DeleteTool(input DeleteToolInput) error {
	return processRequestNoBody(i, "DELETE", i.toolURL("%s", input.NameHash), map[string]string{
		"engine": input.Engine,
	})
}

func (i impl) TouchTool(input TouchToolInput) error {
	return processRequestNoBody(i, "PATCH", i.toolURL("%s/touch", input.NameHash), map[string]string{
		"engine": input.Engine,
	})
}

func (i impl) Ping() error {
	r, err := grequests.Get(i.baseURL+"/v1/ping", &grequests.RequestOptions{
		Headers: map[string]string{
			"Accept":        "application/json",
			"Authorization": "bearer " + i.token,
		},
	})
	if err != nil {
		return err
	}

	if !r.Ok {
		return fmt.Errorf("ping failed with status %d: %s", r.StatusCode, r.String())
	}

	return nil
}

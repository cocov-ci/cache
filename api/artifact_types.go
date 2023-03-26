package api

import "time"

type RegisterArtifactInput struct {
	RepositoryID int64
	Name         string
	NameHash     string
	Size         int64
	Mime         string
	Engine       string
}

type RegisterArtifactOutput struct {
	ID int64 `json:"id"`
}

type GetArtifactMetaInput struct {
	RepositoryID int64
	NameHash     string
	Engine       string
}

type GetArtifactMetaOutput struct {
	ID         int64     `json:"id,omitempty"`
	Name       string    `json:"name,omitempty"`
	NameHash   string    `json:"name_hash,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Engine     string    `json:"engine,omitempty"`
	Mime       string    `json:"mime,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

type DeleteArtifactInput struct {
	RepositoryID int64
	NameHash     string
	Engine       string
}

type TouchArtifactInput struct {
	RepositoryID int64
	NameHash     string
	Engine       string
}

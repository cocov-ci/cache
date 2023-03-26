package api

import "time"

type RegisterToolInput struct {
	Name     string
	NameHash string
	Size     int64
	Mime     string
	Engine   string
}

type RegisterToolOutput struct {
	ID int64 `json:"id"`
}

type GetToolMetaInput struct {
	NameHash string
	Engine   string
}

type GetToolMetaOutput struct {
	ID         int64     `json:"id,omitempty"`
	Name       string    `json:"name"`
	NameHash   string    `json:"name_hash"`
	Size       int64     `json:"size,omitempty"`
	Mime       string    `json:"mime,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

type DeleteToolInput struct {
	NameHash string
	Engine   string
}

type TouchToolInput struct {
	NameHash string
	Engine   string
}

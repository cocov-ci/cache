package housekeeping

import "encoding/json"

type Task struct {
	Name     string `json:"task"`
	ID       string `json:"task_id"`
	original []byte
}

func TaskInto[T any](task *Task) (*T, error) {
	var v T
	err := json.Unmarshal(task.original, &v)
	return &v, err
}

type EvictTask struct {
	Name       string   `json:"task,omitempty"`
	ID         string   `json:"task_id,omitempty"`
	Repository int64    `json:"repository,omitempty"`
	Objects    []string `json:"objects,omitempty"`
}

type PurgeTask struct {
	Name       string `json:"task,omitempty"`
	ID         string `json:"task_id,omitempty"`
	Repository int64  `json:"repository,omitempty"`
}

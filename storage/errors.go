package storage

import "fmt"

type ErrNotExist struct {
	path string
}

func (e ErrNotExist) Error() string {
	return fmt.Sprintf("not found: %s", e.path)
}

type ErrNotGroupable struct {
	path string
}

func (e ErrNotGroupable) Error() string {
	return fmt.Sprintf("object at %s does not support grouping", e.path)
}

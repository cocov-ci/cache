package storage

import "io"

type Provider interface {
	// MetadataOf returns metadata for a given locator
	MetadataOf(locator ObjectDescriptor) (*Item, error)

	// Get obtains an io.ReadCloser for a given locator.
	Get(locator ObjectDescriptor) (*Item, io.ReadCloser, error)

	// Set takes an io.ReadCloser and writes its content to the storage
	Set(locator ObjectDescriptor, mime string, objectSize int, stream io.ReadCloser) error

	// Touch updates the AccessedAt metadata field of a given object under
	// the provided locator.
	Touch(locator ObjectDescriptor) error

	// Delete deletes an object under a given locator
	Delete(locator ObjectDescriptor) error

	// DeleteGroup deletes all the objects under a given object locator.
	DeleteGroup(locator ObjectDescriptor) error

	// TotalSize returns the sum of sizes of all objects under a given kind.
	TotalSize(kind Kind) (int64, error)
}

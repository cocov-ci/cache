package server

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/cocov-ci/cache/logging"
	"github.com/cocov-ci/cache/redis"
	"github.com/cocov-ci/cache/storage"
)

type AuthenticatorFunc func(r *http.Request) error
type DescriptorFunc func(h *Handler, r *http.Request, id string) (storage.ObjectDescriptor, error)

type Handler struct {
	Authenticator    AuthenticatorFunc
	LocatorGenerator DescriptorFunc
	MaxPackageSize   int64
	Provider         storage.Provider
	Redis            redis.Client
}

func (h *Handler) GetRepoName(r *http.Request) (string, error) {
	ok, val, err := h.Redis.RepoNameFromJID(r.Header.Get("Cocov-Job-ID"))
	if err != nil {
		return "", err
	}
	if !ok {
		val = ""
	}
	return val, nil
}

func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	log := logging.GetLogger(r)

	if r.Header.Get("Cocov-Job-ID") == "" {
		log.Error("Rejecting request for lacking a Cocov-Job-ID header")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Missing job identifier"))
		return
	}

	if h.Authenticator != nil {
		if err := h.Authenticator(r); err != nil {
			log.Error("Rejecting request due to authentication failure", zap.Error(err))
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("Forbidden"))
			return
		}
	}

	itemKey := chi.URLParam(r, "id")
	if itemKey == "" {
		log.Error("Rejecting request lacking id")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Missing ID"))
		return
	}

	locator, err := h.LocatorGenerator(h, r, itemKey)
	if err != nil {
		log.Error("Rejecting request due to error returned from LocatorGenerator", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
		return
	}

	var (
		meta   *storage.Item
		reader io.ReadCloser
	)

	err = h.Redis.Locking(locator.PathComponents(), 10*time.Minute, func() error {
		meta, reader, err = h.Provider.Get(locator)
		return err
	})

	if err != nil {
		if _, ok := err.(storage.ErrNotExist); ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not found"))
			return
		}

		log.Error("Rejecting request due to error returned during Locking + Item processing procedure", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
		return
	}

	defer func() { _ = reader.Close() }()

	w.Header().Set("Content-Type", meta.Mime)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.Size))

	_, err = io.Copy(w, reader)
	if err != nil {
		log.Error("Failed transferring data to peer", zap.Error(err))
	}
}

func (h *Handler) HandleHead(w http.ResponseWriter, r *http.Request) {
	log := logging.GetLogger(r)

	if r.Header.Get("Cocov-Job-ID") == "" {
		log.Error("Rejecting request for lacking a Cocov-Job-ID header")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if h.Authenticator != nil {
		if err := h.Authenticator(r); err != nil {
			log.Error("Rejecting request due to authentication failure", zap.Error(err))
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	itemKey := chi.URLParam(r, "id")
	if itemKey == "" {
		log.Error("Rejecting request lacking id")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	locator, err := h.LocatorGenerator(h, r, itemKey)
	if err != nil {
		log.Error("Rejecting request due to error returned from LocatorGenerator", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var (
		meta *storage.Item
	)

	err = h.Redis.Locking(locator.PathComponents(), 10*time.Minute, func() error {
		meta, err = h.Provider.MetadataOf(locator)
		return err
	})

	if err != nil {
		if _, ok := err.(storage.ErrNotExist); ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		log.Error("Rejecting request due to error returned during Locking + Item processing procedure", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.Size))
	w.Header().Set("Content-Type", meta.Mime)

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleSet(w http.ResponseWriter, r *http.Request) {
	log := logging.GetLogger(r)

	if r.Header.Get("Cocov-Job-ID") == "" {
		log.Error("Rejecting request for lacking a Cocov-Job-ID header")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Missing job identifier"))
		return
	}

	if h.Authenticator != nil {
		if err := h.Authenticator(r); err != nil {
			log.Error("Rejecting request due to authentication failure", zap.Error(err))
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("Forbidden"))
			return
		}
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		log.Error("Rejecting request lacking Content-Type")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Missing Content-Type"))
		return
	}

	if r.ContentLength <= 0 {
		log.Error("Rejecting request with empty Content-Length header")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Missing Content-Length"))
		return
	}

	if h.MaxPackageSize != 0 {
		if r.ContentLength > h.MaxPackageSize {
			log.Error("Request body is larger than the maximum configured value",
				zap.Int64("max_package_size", h.MaxPackageSize),
				zap.Int64("received_content_size", r.ContentLength))
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte("Request entity too large"))
			return
		}
	}

	itemKey := chi.URLParam(r, "id")
	if itemKey == "" {
		log.Error("Rejecting request lacking id")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Missing ID"))

		return
	}

	locator, err := h.LocatorGenerator(h, r, itemKey)
	if err != nil {
		log.Error("Rejecting request due to error returned from LocatorGenerator", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
		return
	}

	err = h.Redis.Locking(locator.PathComponents(), 10*time.Minute, func() error {
		defer func(body io.ReadCloser) { _ = body.Close() }(r.Body)
		return h.Provider.Set(locator, contentType, int(r.ContentLength), io.NopCloser(io.LimitReader(r.Body, r.ContentLength)))
	})

	if err != nil {
		log.Error("Error writing data to storage", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

package server

import (
	"fmt"
	"github.com/cocov-ci/cache/locator"
	"io"
	"net/http"
	"reflect"
	"time"

	"go.uber.org/zap"

	"github.com/cocov-ci/cache/logging"
	"github.com/cocov-ci/cache/redis"
	"github.com/cocov-ci/cache/storage"
)

type AuthenticatorFunc func(r *http.Request) error

type Handler[L locator.Locator] struct {
	Authenticator  AuthenticatorFunc
	MaxPackageSize int64
	Redis          redis.Client

	HandleGet  func(loc L, r *http.Request) (size int64, mime string, stream io.ReadCloser, err error)
	HandleHead func(loc L, r *http.Request) (size int64, mime string, err error)
	HandleSet  func(loc L, mime string, size int64, stream io.ReadCloser) error
}

func (h *Handler[L]) makeLocator() L {
	var rawLocator L
	t := reflect.TypeOf(rawLocator)
	i := reflect.New(t.Elem())
	return i.Interface().(L)
}

func (h *Handler[L]) Get(w http.ResponseWriter, r *http.Request) {
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

	var loc = h.makeLocator()
	if err := loc.LoadFrom(h.Redis, r); err != nil {
		if httpErr, ok := err.(locator.HTTPError); ok {
			log.Error("Locator rejecting with custom HTTP error", zap.Error(httpErr))
			w.WriteHeader(httpErr.Status)
			_, _ = w.Write([]byte(httpErr.Message))
		} else {
			log.Error("Rejecting request due to error returned from locator", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal server error"))
		}
		return
	}

	var (
		mime   string
		size   int64
		reader io.ReadCloser
		err    error
	)

	err = h.Redis.Locking(loc.LockKey(), 10*time.Minute, func() error {
		size, mime, reader, err = h.HandleGet(loc, r)
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

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

	_, err = io.Copy(w, reader)
	if err != nil {
		log.Error("Failed transferring data to peer", zap.Error(err))
	}
}

func (h *Handler[L]) Head(w http.ResponseWriter, r *http.Request) {
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

	loc := h.makeLocator()
	if err := loc.LoadFrom(h.Redis, r); err != nil {
		if httpErr, ok := err.(locator.HTTPError); ok {
			log.Error("Locator rejecting with custom HTTP error", zap.Error(httpErr))
			w.WriteHeader(httpErr.Status)
			_, _ = w.Write([]byte(httpErr.Message))
		} else {
			log.Error("Rejecting request due to error returned from locator", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal server error"))
		}
		return
	}

	var (
		mime string
		size int64
		err  error
	)

	err = h.Redis.Locking(loc.LockKey(), 10*time.Minute, func() error {
		size, mime, err = h.HandleHead(loc, r)
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

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.WriteHeader(http.StatusOK)
}

func (h *Handler[L]) Set(w http.ResponseWriter, r *http.Request) {
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

	itemName := r.Header.Get("Filename")
	if itemName == "" {
		log.Error("Rejecting request lacking filename")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Missing Filename"))
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

	loc := h.makeLocator()
	if err := loc.LoadFrom(h.Redis, r); err != nil {
		if httpErr, ok := err.(locator.HTTPError); ok {
			log.Error("Locator rejecting with custom HTTP error", zap.Error(httpErr))
			w.WriteHeader(httpErr.Status)
			_, _ = w.Write([]byte(httpErr.Message))
		} else {
			log.Error("Rejecting request due to error returned from locator", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal server error"))
		}
		return
	}

	err := h.Redis.Locking(loc.LockKey(), 10*time.Minute, func() error {
		defer func(body io.ReadCloser) { _ = body.Close() }(r.Body)
		return h.HandleSet(loc, contentType, r.ContentLength, io.NopCloser(io.LimitReader(r.Body, r.ContentLength)))
	})

	if err != nil {
		log.Error("Error writing data to storage", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

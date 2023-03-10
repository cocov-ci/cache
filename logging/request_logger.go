package logging

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type contextKey struct {
	name string
}

func (k *contextKey) String() string {
	return "cocov/cache/logging context value " + k.name
}

var loggerKey = &contextKey{"logger"}

func RequestLogger(log *zap.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			rid := uuid.NewString()
			l := log.With(zap.String("request_id", rid))

			w.Header().Add("Cocov-Request-ID", rid)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			t1 := time.Now()
			defer func() {
				l.Info("Request completed", zap.String("method", r.Method), zap.String("path", r.URL.Path), zap.Int("status", ww.Status()), zap.Int("bytes_written", ww.BytesWritten()), zap.Duration("duration", time.Since(t1)))
			}()

			next.ServeHTTP(ww, r.WithContext(context.WithValue(r.Context(), loggerKey, l)))
		}

		return http.HandlerFunc(fn)
	}
}

func GetLogger(r *http.Request) *zap.Logger {
	raw := r.Context().Value(loggerKey)
	if raw == nil {
		return nil
	}

	return raw.(*zap.Logger)
}

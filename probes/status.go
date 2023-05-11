package probes

import (
	"net/http"
	"sync/atomic"
	"time"
)

var (
	ok          = []byte("OK")
	unavailable = []byte("Unavailable")
)

type Status struct {
	server *http.Server
	ready  *atomic.Bool
}

func NewStatus(bindAddress string) *Status {
	s := &Status{
		ready: &atomic.Bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/system/probes/liveness", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(ok)
	})
	mux.HandleFunc("/system/probes/readiness", func(w http.ResponseWriter, r *http.Request) {
		if !s.ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write(unavailable)
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(ok)
		}
	})

	srv := http.Server{
		Addr:              bindAddress,
		Handler:           mux,
		ReadTimeout:       2 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       10 * time.Second,
	}

	s.server = &srv

	return s
}

func (s *Status) Run() error { return s.server.ListenAndServe() }

func (s *Status) SetReady(ready bool) {
	s.ready.Store(ready)
}

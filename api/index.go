package handler

import (
	"net/http"
	"strings"
	"sync"

	"github.com/kudanilll/favget/pkg/app"
)

var (
	once    sync.Once
	h       http.Handler
	initErr error
)

func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(func() { h, initErr = app.NewHandler() })
	if initErr != nil {
		http.Error(w, "init error: "+initErr.Error(), http.StatusInternalServerError)
		return
	}

	// CORS preflight sederhana
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	r2 := r.Clone(r.Context())
	r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")

	h.ServeHTTP(w, r2)
}

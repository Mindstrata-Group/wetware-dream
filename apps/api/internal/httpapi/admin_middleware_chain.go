package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

type adminActorContextKey struct{}

type adminMiddleware func(http.HandlerFunc) http.HandlerFunc

type adminHandlerChain struct {
	h           Handler
	middlewares []adminMiddleware
}

func (h Handler) WithAdminAuth(section string, mutating bool) adminHandlerChain {
	return adminHandlerChain{h: h}.WithAdminAuth(section, mutating)
}

func (c adminHandlerChain) WithAdminAuth(section string, mutating bool) adminHandlerChain {
	c.middlewares = append(c.middlewares, func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, ok := c.h.requireAdminSection(w, r, section, mutating)
			if !ok {
				return
			}
			ctx := context.WithValue(r.Context(), adminActorContextKey{}, actor)
			next(w, r.WithContext(ctx))
		}
	})
	return c
}

func (c adminHandlerChain) WithCache(cache *simpleGlobalCache, ttl time.Duration) adminHandlerChain {
	c.middlewares = append(c.middlewares, func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && writeCached(w, cache) {
				return
			}
			capture := &cacheResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next(capture, r)
			if r.Method == http.MethodGet && capture.status >= 200 && capture.status < 300 && capture.body.Len() > 0 {
				cache.set(capture.body.Bytes(), ttl)
			}
		}
	})
	return c
}

func (c adminHandlerChain) Handler(logic http.HandlerFunc) http.HandlerFunc {
	wrapped := logic
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		wrapped = c.middlewares[i](wrapped)
	}
	return wrapped
}

type cacheResponseWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *cacheResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *cacheResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.status >= 200 && w.status < 300 {
		_, _ = w.body.Write(body)
	}
	return w.ResponseWriter.Write(body)
}

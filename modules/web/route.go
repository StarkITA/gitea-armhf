// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package web

import (
	goctx "context"
	"fmt"
	"net/http"
	"strings"

	"code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/web/middleware"

	"gitea.com/go-chi/binding"
	chi "github.com/go-chi/chi/v5"
)

// Bind binding an obj to a handler
func Bind[T any](obj T) http.HandlerFunc {
	return Wrap(func(ctx *context.Context) {
		theObj := new(T) // create a new form obj for every request but not use obj directly
		binding.Bind(ctx.Req, theObj)
		SetForm(ctx, theObj)
		middleware.AssignForm(theObj, ctx.Data)
	})
}

// SetForm set the form object
func SetForm(data middleware.DataStore, obj interface{}) {
	data.GetData()["__form"] = obj
}

// GetForm returns the validate form information
func GetForm(data middleware.DataStore) interface{} {
	return data.GetData()["__form"]
}

// Route defines a route based on chi's router
type Route struct {
	R              chi.Router
	curGroupPrefix string
	curMiddlewares []interface{}
}

// NewRoute creates a new route
func NewRoute() *Route {
	r := chi.NewRouter()
	return &Route{
		R:              r,
		curGroupPrefix: "",
		curMiddlewares: []interface{}{},
	}
}

// Use supports two middlewares
func (r *Route) Use(middlewares ...interface{}) {
	if r.curGroupPrefix != "" {
		r.curMiddlewares = append(r.curMiddlewares, middlewares...)
	} else {
		for _, middle := range middlewares {
			switch t := middle.(type) {
			case func(http.Handler) http.Handler:
				r.R.Use(t)
			case func(*context.Context):
				r.R.Use(Middle(t))
			case func(*context.Context) goctx.CancelFunc:
				r.R.Use(MiddleCancel(t))
			case func(*context.APIContext):
				r.R.Use(MiddleAPI(t))
			default:
				panic(fmt.Sprintf("Unsupported middleware type: %#v", t))
			}
		}
	}
}

// Group mounts a sub-Router along a `pattern` string.
func (r *Route) Group(pattern string, fn func(), middlewares ...interface{}) {
	previousGroupPrefix := r.curGroupPrefix
	previousMiddlewares := r.curMiddlewares
	r.curGroupPrefix += pattern
	r.curMiddlewares = append(r.curMiddlewares, middlewares...)

	fn()

	r.curGroupPrefix = previousGroupPrefix
	r.curMiddlewares = previousMiddlewares
}

func (r *Route) getPattern(pattern string) string {
	newPattern := r.curGroupPrefix + pattern
	if !strings.HasPrefix(newPattern, "/") {
		newPattern = "/" + newPattern
	}
	if newPattern == "/" {
		return newPattern
	}
	return strings.TrimSuffix(newPattern, "/")
}

// Mount attaches another Route along ./pattern/*
func (r *Route) Mount(pattern string, subR *Route) {
	middlewares := make([]interface{}, len(r.curMiddlewares))
	copy(middlewares, r.curMiddlewares)
	subR.Use(middlewares...)
	r.R.Mount(r.getPattern(pattern), subR.R)
}

// Any delegate requests for all methods
func (r *Route) Any(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.HandleFunc(r.getPattern(pattern), Wrap(middlewares...))
}

// Route delegate special methods
func (r *Route) Route(pattern, methods string, h ...interface{}) {
	p := r.getPattern(pattern)
	ms := strings.Split(methods, ",")
	middlewares := r.getMiddlewares(h)
	for _, method := range ms {
		r.R.MethodFunc(strings.TrimSpace(method), p, Wrap(middlewares...))
	}
}

// Delete delegate delete method
func (r *Route) Delete(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Delete(r.getPattern(pattern), Wrap(middlewares...))
}

func (r *Route) getMiddlewares(h []interface{}) []interface{} {
	middlewares := make([]interface{}, len(r.curMiddlewares), len(r.curMiddlewares)+len(h))
	copy(middlewares, r.curMiddlewares)
	middlewares = append(middlewares, h...)
	return middlewares
}

// Get delegate get method
func (r *Route) Get(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Get(r.getPattern(pattern), Wrap(middlewares...))
}

// Options delegate options method
func (r *Route) Options(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Options(r.getPattern(pattern), Wrap(middlewares...))
}

// GetOptions delegate get and options method
func (r *Route) GetOptions(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Get(r.getPattern(pattern), Wrap(middlewares...))
	r.R.Options(r.getPattern(pattern), Wrap(middlewares...))
}

// PostOptions delegate post and options method
func (r *Route) PostOptions(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Post(r.getPattern(pattern), Wrap(middlewares...))
	r.R.Options(r.getPattern(pattern), Wrap(middlewares...))
}

// Head delegate head method
func (r *Route) Head(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Head(r.getPattern(pattern), Wrap(middlewares...))
}

// Post delegate post method
func (r *Route) Post(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Post(r.getPattern(pattern), Wrap(middlewares...))
}

// Put delegate put method
func (r *Route) Put(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Put(r.getPattern(pattern), Wrap(middlewares...))
}

// Patch delegate patch method
func (r *Route) Patch(pattern string, h ...interface{}) {
	middlewares := r.getMiddlewares(h)
	r.R.Patch(r.getPattern(pattern), Wrap(middlewares...))
}

// ServeHTTP implements http.Handler
func (r *Route) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.R.ServeHTTP(w, req)
}

// NotFound defines a handler to respond whenever a route could
// not be found.
func (r *Route) NotFound(h http.HandlerFunc) {
	r.R.NotFound(h)
}

// MethodNotAllowed defines a handler to respond whenever a method is
// not allowed.
func (r *Route) MethodNotAllowed(h http.HandlerFunc) {
	r.R.MethodNotAllowed(h)
}

// Combo deletegate requests to Combo
func (r *Route) Combo(pattern string, h ...interface{}) *Combo {
	return &Combo{r, pattern, h}
}

// Combo represents a tiny group routes with same pattern
type Combo struct {
	r       *Route
	pattern string
	h       []interface{}
}

// Get deletegate Get method
func (c *Combo) Get(h ...interface{}) *Combo {
	c.r.Get(c.pattern, append(c.h, h...)...)
	return c
}

// Post deletegate Post method
func (c *Combo) Post(h ...interface{}) *Combo {
	c.r.Post(c.pattern, append(c.h, h...)...)
	return c
}

// Delete deletegate Delete method
func (c *Combo) Delete(h ...interface{}) *Combo {
	c.r.Delete(c.pattern, append(c.h, h...)...)
	return c
}

// Put deletegate Put method
func (c *Combo) Put(h ...interface{}) *Combo {
	c.r.Put(c.pattern, append(c.h, h...)...)
	return c
}

// Patch deletegate Patch method
func (c *Combo) Patch(h ...interface{}) *Combo {
	c.r.Patch(c.pattern, append(c.h, h...)...)
	return c
}

package main

import (
	"net/http"

	"github.com/martini-contrib/cors"
)

func ApplicationJSON(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		h.ServeHTTP(w, r)
	})
}

func CORS(h http.Handler) http.Handler {
	f := cors.Allow(&cors.Options{
		AllowAllOrigins: true,
		AllowMethods:    []string{"PUT", "PATCH", "DELETE", "POST"},
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f(w, r)
		h.ServeHTTP(w, r)
	})
}

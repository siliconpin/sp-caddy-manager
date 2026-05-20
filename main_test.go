package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sp-caddy-manager/functions"
)

func TestNewRouterDoesNotServeHTMLWithoutDevMode(t *testing.T) {
	router := newRouter(&functions.App{}, false)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestNewRouterServesHTMLInDevMode(t *testing.T) {
	router := newRouter(&functions.App{}, true)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestIsDevMode(t *testing.T) {
	if !isDevMode([]string{"dev"}) {
		t.Fatalf("expected dev argument to enable dev mode")
	}
	if isDevMode(nil) {
		t.Fatalf("expected no arguments to disable dev mode")
	}
}

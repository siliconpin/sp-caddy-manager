package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestEnsurePlaceholderCaddyfileCreatesParsablePlaceholder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.caddy")

	if err := ensurePlaceholderCaddyfile(path); err != nil {
		t.Fatalf("ensure placeholder failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read placeholder failed: %v", err)
	}
	if string(content) != placeholderCaddyfileContent {
		t.Fatalf("unexpected placeholder content: %q", string(content))
	}
}

func TestEnsurePlaceholderCaddyfileReplacesBlankFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.caddy")
	if err := os.WriteFile(path, []byte("\n"), 0644); err != nil {
		t.Fatalf("write blank placeholder failed: %v", err)
	}

	if err := ensurePlaceholderCaddyfile(path); err != nil {
		t.Fatalf("ensure placeholder failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read placeholder failed: %v", err)
	}
	if string(content) != placeholderCaddyfileContent {
		t.Fatalf("expected blank file to be replaced, got: %q", string(content))
	}
}

func TestEnsurePlaceholderCaddyfilePreservesNonBlankFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.caddy")
	original := "# custom placeholder\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("write placeholder failed: %v", err)
	}

	if err := ensurePlaceholderCaddyfile(path); err != nil {
		t.Fatalf("ensure placeholder failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read placeholder failed: %v", err)
	}
	if string(content) != original {
		t.Fatalf("expected non-blank file to be preserved, got: %q", string(content))
	}
}

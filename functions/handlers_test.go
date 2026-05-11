package functions

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// Mock database setup for testing
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create domains table
	_, err = db.Exec(`
		CREATE TABLE domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT UNIQUE NOT NULL,
			port INTEGER NOT NULL,
			content TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted INTEGER DEFAULT 0,
			ssl INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	return db
}

// Test response helper functions
func TestWriteJSONResponse(t *testing.T) {
	w := httptest.NewRecorder()

	writeJSONResponse(w, "test.com", true, "test error", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["domain"] != "test.com" {
		t.Errorf("Expected domain 'test.com', got '%v'", response["domain"])
	}

	if response["err"] != true {
		t.Errorf("Expected err true, got %v", response["err"])
	}

	if response["msg"] != "test error" {
		t.Errorf("Expected msg 'test error', got '%v'", response["msg"])
	}
}

func TestWriteErrorResponse(t *testing.T) {
	w := httptest.NewRecorder()

	writeErrorResponse(w, "test.com", "test error", http.StatusInternalServerError)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["err"] != true {
		t.Errorf("Expected err true, got %v", response["err"])
	}
}

func TestWriteSuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()

	writeSuccessResponse(w, "test.com", "success message")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["err"] != false {
		t.Errorf("Expected err false, got %v", response["err"])
	}
}

func TestWithTransaction(t *testing.T) {
	db := setupTestDB(t)

	err := withTransaction(db, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO domains (domain, port, content, created_at, updated_at, deleted, ssl) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"test.com", 8080, "test content", "2023-01-01T00:00:00Z", "2023-01-01T00:00:00Z", 0, 0)
		return err
	})

	if err != nil {
		t.Errorf("Transaction failed: %v", err)
	}

	// Verify data was committed
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM domains WHERE domain = ?", "test.com").Scan(&count)
	if err != nil {
		t.Errorf("Query failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}

func TestCheckDomainExists(t *testing.T) {
	db := setupTestDB(t)
	app := &App{DB: db}

	// Test non-existent domain
	exists, err := app.checkDomainExists("nonexistent.com")
	if err != nil {
		t.Errorf("checkDomainExists failed: %v", err)
	}
	if exists {
		t.Errorf("Expected false for non-existent domain, got true")
	}

	// Insert a domain
	_, err = db.Exec("INSERT INTO domains (domain, port, content, created_at, updated_at, deleted, ssl) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test.com", 8080, "test content", "2023-01-01T00:00:00Z", "2023-01-01T00:00:00Z", 0, 0)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Test existing domain
	exists, err = app.checkDomainExists("test.com")
	if err != nil {
		t.Errorf("checkDomainExists failed: %v", err)
	}
	if !exists {
		t.Errorf("Expected true for existing domain, got false")
	}
}

func TestHandleManageDomain_MethodNotAllowed(t *testing.T) {
	db := setupTestDB(t)
	app := &App{DB: db}

	// Test GET request (should fail)
	req := httptest.NewRequest(http.MethodGet, "/manage-domain", nil)
	w := httptest.NewRecorder()

	app.HandleManageDomain(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["err"] != true {
		t.Errorf("Expected err true, got %v", response["err"])
	}
}

func TestCaddyDomainEntriesExtractsPortFromNestedSubroute(t *testing.T) {
	route := map[string]interface{}{
		"match": []interface{}{
			map[string]interface{}{"host": []interface{}{"app.example.com"}},
		},
		"handle": []interface{}{
			map[string]interface{}{
				"handler": "subroute",
				"routes": []interface{}{
					map[string]interface{}{
						"handle": []interface{}{
							map[string]interface{}{
								"handler": "reverse_proxy",
								"upstreams": []interface{}{
									map[string]interface{}{"dial": "localhost:8080"},
								},
							},
						},
					},
				},
			},
		},
	}

	entries := extractDomainEntriesFromRoute(route)
	if len(entries) != 1 {
		t.Fatalf("expected one domain entry, got %#v", entries)
	}
	if entries[0].Domain != "app.example.com" || entries[0].Port != 8080 {
		t.Fatalf("unexpected domain entry: %#v", entries[0])
	}
}

func TestCaddyDomainEntriesExtractsPortFromLaterUpstream(t *testing.T) {
	route := map[string]interface{}{
		"match": []interface{}{
			map[string]interface{}{"host": []interface{}{"app.example.com"}},
		},
		"handle": []interface{}{
			map[string]interface{}{
				"handler": "reverse_proxy",
				"upstreams": []interface{}{
					map[string]interface{}{"dial": "unix//run/backend.sock"},
					map[string]interface{}{"dial": "127.0.0.1:9090"},
				},
			},
		},
	}

	entries := extractDomainEntriesFromRoute(route)
	if len(entries) != 1 {
		t.Fatalf("expected one domain entry, got %#v", entries)
	}
	if entries[0].Port != 9090 {
		t.Fatalf("expected port 9090, got %#v", entries[0])
	}
}

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

type DomainRequest struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
}

func main() {
	var err error
	db, err = sql.Open("sqlite3", "./domains.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create table if not exists
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS domains (
		domain TEXT PRIMARY KEY,
		port INTEGER
	)`)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/add-domain", handleAddDomain)
	http.HandleFunc("/delete-domain", handleDeleteDomain)
	http.HandleFunc("/version", handleVersion)
	http.HandleFunc("/health", handleHealth)
	fmt.Println("Server starting on :3000...")
	http.ListenAndServe(":3000", nil)
}

func handleAddDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Create the Caddyfile snippet for persistence
	caddyfileContent := fmt.Sprintf("%s {\n\treverse_proxy localhost:%d\n}\n", req.Domain, req.Port)
	filePath := filepath.Join("/etc/caddy/conf.d", req.Domain+".caddy")
	
	if err := os.WriteFile(filePath, []byte(caddyfileContent), 0644); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Push to Caddy JSON API for instant activation
	// Note: We use the POST API to append a new route to Caddy's default server 'srv0'
	caddyRoute := map[string]interface{}{
		"match": []map[string]interface{}{
			{"host": []string{req.Domain}},
		},
		"handle": []map[string]interface{}{
			{
				"handler": "reverse_proxy",
				"upstreams": []map[string]string{
					{"dial": fmt.Sprintf("localhost:%d", req.Port)},
				},
			},
		},
		"terminal": true,
	}

	jsonPayload, _ := json.Marshal(caddyRoute)
	// Path assumes standard Caddy structure: apps -> http -> servers -> srv0 -> routes
	apiURL := "http://localhost:2019/config/apps/http/servers/srv0/routes"
	
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil || resp.StatusCode >= 400 {
		http.Error(w, "Caddy API update failed", http.StatusInternalServerError)
		return
	}

	// Insert into database
	_, err = db.Exec("INSERT INTO domains (domain, port) VALUES (?, ?)", req.Domain, req.Port)
	if err != nil {
		http.Error(w, "Failed to save to database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Domain %s added successfully and saved to %s", req.Domain, filePath)
}

func handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if domain exists in DB
	var port int
	err := db.QueryRow("SELECT port FROM domains WHERE domain = ?", req.Domain).Scan(&port)
	if err == sql.ErrNoRows {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove the file
	filePath := filepath.Join("/etc/caddy/conf.d", req.Domain+".caddy")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		http.Error(w, "Failed to remove file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove from database
	_, err = db.Exec("DELETE FROM domains WHERE domain = ?", req.Domain)
	if err != nil {
		http.Error(w, "Failed to delete from database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove from Caddy config
	// First, get current routes
	resp, err := http.Get("http://localhost:2019/config/apps/http/servers/srv0/routes")
	if err != nil {
		http.Error(w, "Failed to get Caddy config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var routes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		http.Error(w, "Failed to decode Caddy config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter out the route for this domain
	var newRoutes []map[string]interface{}
	for _, route := range routes {
		match, ok := route["match"].([]interface{})
		if !ok {
			newRoutes = append(newRoutes, route)
			continue
		}
		skip := false
		for _, m := range match {
			mm, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			host, ok := mm["host"].([]interface{})
			if !ok {
				continue
			}
			for _, h := range host {
				if h == req.Domain {
					skip = true
					break
				}
			}
			if skip {
				break
			}
		}
		if !skip {
			newRoutes = append(newRoutes, route)
		}
	}

	// PUT the updated routes
	jsonPayload, _ := json.Marshal(newRoutes)
	req2, err := http.NewRequest("PUT", "http://localhost:2019/config/apps/http/servers/srv0/routes", bytes.NewBuffer(jsonPayload))
	if err != nil {
		http.Error(w, "Failed to create request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil || resp2.StatusCode >= 400 {
		http.Error(w, "Caddy config update failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Domain %s deleted successfully", req.Domain)
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	version := "1.0.0"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": version})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

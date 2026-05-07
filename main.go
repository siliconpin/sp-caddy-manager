package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var (
	db             *sql.DB
	caddyConfigDir = "/etc/caddy/conf.d"
	caddyAPIURL    = "http://localhost:2019/config/apps/http/servers/srv0/routes"
)

type DomainRequest struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
}

func main() {
	loadDotEnv(".env")

	port := getPortFromEnv()
	caddyConfigDir = getCaddyConfigDir()
	caddyAPIURL = getCaddyAPIURL()

	// Check if caddy command is available
	cmd := exec.Command("which", "caddy")
	out, err := cmd.Output()
	if err != nil {
		log.Println("WARNING: caddy command not found in PATH")
	} else {
		log.Printf("INFO: caddy command is available at: %s", strings.TrimSpace(string(out)))
	}

	// Create caddy config directory
	if err := os.MkdirAll(caddyConfigDir, 0744); err != nil {
		log.Fatalf("Failed to create caddy config directory: %v", err)
	}
	log.Printf("INFO: Caddy config directory ensured: %s", caddyConfigDir)

	// Create empty.caddy file if it doesn't exist
	emptyFileName := filepath.Join(caddyConfigDir, "empty.caddy")
	if _, err := os.Stat(emptyFileName); os.IsNotExist(err) {
		if err := os.WriteFile(emptyFileName, []byte(""), 0744); err != nil {
			log.Fatalf("Failed to create empty.caddy file: %v", err)
		}
		log.Printf("INFO: Created empty.caddy file at %s", emptyFileName)
	}

	var dbErr error
	db, dbErr = sql.Open("sqlite3", getDBPath())
	if dbErr != nil {
		log.Fatal(dbErr)
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

	fmt.Printf("Server starting on :%d...\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), newRouter()))
}

func newRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/add-domain", handleAddDomain)
	mux.HandleFunc("/delete-domain", handleDeleteDomain)
	mux.HandleFunc("/api/domains", handleListDomains)
	mux.HandleFunc("/version", handleVersion)
	mux.HandleFunc("/health", handleHealth)
	mux.Handle("/", http.FileServer(http.Dir("./html")))
	return mux
}

func getEnvDefault(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

func getDBPath() string {
	return getEnvDefault("DB_PATH", "./domains.sqlite")
}

func getCaddyConfigDir() string {
	return getEnvDefault("CADDY_CONFIG_DIR", "/etc/caddy/conf.d")
}

func getCaddyAPIURL() string {
	return getEnvDefault("CADDY_API_URL", "http://localhost:2019/config/apps/http/servers/srv0/routes")
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}

		os.Setenv(key, strings.Trim(value, `"`))
	}
}

func getPortFromEnv() int {
	portStr := os.Getenv("PORT")
	if portStr == "" {
		return 3000
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("invalid PORT value %q: %v", portStr, err)
	}
	return port
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
	filePath := filepath.Join(caddyConfigDir, req.Domain+".caddy")
	
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
	resp, err := http.Post(caddyAPIURL, "application/json", bytes.NewBuffer(jsonPayload))
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
	filePath := filepath.Join(caddyConfigDir, req.Domain+".caddy")
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
	resp, err := http.Get(caddyAPIURL)
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
	req2, err := http.NewRequest("PUT", caddyAPIURL, bytes.NewBuffer(jsonPayload))
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

func handleListDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query("SELECT domain, port FROM domains ORDER BY domain")
	if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type domainEntry struct {
		Domain string `json:"domain"`
		Port   int    `json:"port"`
	}

	var entries []domainEntry
	for rows.Next() {
		var e domainEntry
		if err := rows.Scan(&e.Domain, &e.Port); err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		entries = append(entries, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
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

package functions

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var DB *sql.DB
var CaddyConfigDir string
var CaddyAPIURL string

func HandleManageDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Route based on action
	switch req.Action {
	case "add":
		handleAddDomainAction(w, r, req)
	case "delete":
		handleDeleteDomainAction(w, r, req)
	case "add-caddyfile":
		handleAddCaddyfileAction(w, r, req)
	default:
		handleAddDomainAction(w, r, req)
	}
}

func HandleAddDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	handleAddDomainAction(w, r, req)
}

func HandleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	handleDeleteDomainAction(w, r, req)
}

func handleAddDomainAction(w http.ResponseWriter, r *http.Request, req DomainRequest) {
	if req.Domain == "" {
		http.Error(w, "domain field is required", http.StatusBadRequest)
		return
	}
	if req.Port == 0 {
		http.Error(w, "port field is required", http.StatusBadRequest)
		return
	}

	// 1. Create the Caddyfile snippet for persistence
	caddyfileContent := fmt.Sprintf("%s {\n\treverse_proxy localhost:%d\n}\n", req.Domain, req.Port)
	filePath := filepath.Join(CaddyConfigDir, req.Domain+".caddy")

	if err := os.WriteFile(filePath, []byte(caddyfileContent), 0644); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Push to Caddy JSON API for instant activation
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
	resp, err := http.Post(CaddyAPIURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			resp.Body.Close()
		}
		http.Error(w, "Caddy API update failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Insert into database
	_, err = DB.Exec("INSERT INTO domains (domain, port) VALUES (?, ?)", req.Domain, req.Port)
	if err != nil {
		http.Error(w, "Failed to save to database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Domain %s added successfully and saved to %s", req.Domain, filePath)
}

func handleAddCaddyfileAction(w http.ResponseWriter, r *http.Request, req DomainRequest) {
	if req.Content == "" {
		http.Error(w, "content field is required for add-caddyfile action", http.StatusBadRequest)
		return
	}

	// Extract domain from content (first line typically contains domain)
	lines := strings.Split(req.Content, "\n")
	if len(lines) == 0 {
		http.Error(w, "content is empty", http.StatusBadRequest)
		return
	}

	// Parse domain from first line (e.g., "domain.com {" -> "domain.com")
	firstLine := strings.TrimSpace(lines[0])
	fields := strings.Fields(firstLine)
	if len(fields) == 0 {
		http.Error(w, "could not extract domain from content", http.StatusBadRequest)
		return
	}
	domain := fields[0]

	if domain == "" {
		http.Error(w, "could not extract domain from content", http.StatusBadRequest)
		return
	}

	// Extract port from reverse_proxy line
	var port int
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "reverse_proxy") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				addr := parts[1]
				if colonIdx := strings.LastIndex(addr, ":"); colonIdx != -1 {
					portStr := addr[colonIdx+1:]
					parsedPort, err := strconv.Atoi(portStr)
					if err == nil {
						port = parsedPort
						break
					}
				}
			}
		}
	}

	if port == 0 {
		http.Error(w, "could not extract port from reverse_proxy line", http.StatusBadRequest)
		return
	}

	filename := filepath.Join(CaddyConfigDir, domain+".caddy")

	if err := os.WriteFile(filename, []byte(req.Content), 0644); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update Caddy config
	if err := CaddyConfigUpdate(filepath.Join(CaddyConfigDir, "Caddyfile")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert into database
	_, err := DB.Exec("INSERT INTO domains (domain, port) VALUES (?, ?)", domain, port)
	if err != nil {
		http.Error(w, "Failed to save to database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Caddyfile content added successfully for domain %s (port %d) and saved to %s", domain, port, filename)
}

func handleDeleteDomainAction(w http.ResponseWriter, r *http.Request, req DomainRequest) {
	if req.Domain == "" {
		http.Error(w, "domain field is required", http.StatusBadRequest)
		return
	}

	// Check if domain exists in DB
	var port int
	err := DB.QueryRow("SELECT port FROM domains WHERE domain = ?", req.Domain).Scan(&port)
	if err == sql.ErrNoRows {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if domain file exists in Caddy config
	filePath := filepath.Join(CaddyConfigDir, req.Domain+".caddy")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		http.Error(w, "Failed to remove file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = DB.Exec("DELETE FROM domains WHERE domain = ?", req.Domain)
	if err != nil {
		http.Error(w, "Failed to delete from database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := removeDomainFromCaddyAPI(req.Domain); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Domain %s deleted successfully", req.Domain)
}

func removeDomainFromCaddyAPI(domain string) error {
	resp, err := http.Get(CaddyAPIURL)
	if err != nil {
		return fmt.Errorf("failed to get Caddy config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to get Caddy config: status %d", resp.StatusCode)
	}

	var routes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return fmt.Errorf("failed to decode Caddy config: %v", err)
	}

	newRoutes := routes[:0]
	for _, route := range routes {
		if routeMatchesDomain(route, domain) {
			continue
		}
		newRoutes = append(newRoutes, route)
	}

	jsonPayload, err := json.Marshal(newRoutes)
	if err != nil {
		return fmt.Errorf("failed to encode Caddy config: %v", err)
	}

	req, err := http.NewRequest(http.MethodPut, CaddyAPIURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create Caddy config request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update Caddy config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Caddy config update failed: status %d", resp.StatusCode)
	}

	return nil
}

func routeMatchesDomain(route map[string]interface{}, domain string) bool {
	match, ok := route["match"].([]interface{})
	if !ok {
		return false
	}

	for _, item := range match {
		matcher, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		hosts, ok := matcher["host"].([]interface{})
		if !ok {
			continue
		}

		for _, host := range hosts {
			if host == domain {
				return true
			}
		}
	}

	return false
}

func HandleListDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := DB.Query("SELECT domain, port FROM domains ORDER BY domain")
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

func HandleVersion(w http.ResponseWriter, r *http.Request) {
	version := "1.0.0"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": version})
}

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

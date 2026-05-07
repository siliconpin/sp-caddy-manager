package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

type DomainRequest struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
}

func main() {
	http.HandleFunc("/add-domain", handleAddDomain)
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

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Domain %s added successfully and saved to %s", req.Domain, filePath)
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

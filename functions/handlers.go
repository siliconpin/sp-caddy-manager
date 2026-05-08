package functions

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const caddyRequestTimeout = 5 * time.Second

var domainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type App struct {
	DB             *sql.DB
	CaddyConfigDir string
	CaddyAPIURL    string
	CaddyfilePath  string
	HTTPClient     *http.Client
}

type domainEntry struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
}

func NewApp(db *sql.DB, caddyConfigDir, caddyAPIURL, caddyfilePath string) *App {
	return &App{
		DB:             db,
		CaddyConfigDir: caddyConfigDir,
		CaddyAPIURL:    caddyAPIURL,
		CaddyfilePath:  caddyfilePath,
		HTTPClient:     &http.Client{Timeout: caddyRequestTimeout},
	}
}

func (a *App) HandleManageDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch req.Action {
	case "add":
		a.handleAddDomainAction(w, req)
	case "delete":
		a.handleDeleteDomainAction(w, req)
	case "add-caddyfile":
		a.handleAddCaddyfileAction(w, req)
	case "list-db":
		a.handleListDomainsAction(w)
	case "list-db-with-content":
		a.handleListDBWithContentAction(w)
	case "list-caddy":
		a.handleListCaddyDomainsAction(w)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

type dbEntry struct {
	ID        int    `json:"id"`
	Domain    string `json:"domain"`
	Port      int    `json:"port"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (a *App) handleListDBWithContentAction(w http.ResponseWriter) {
	rows, err := a.DB.Query("SELECT id, domain, port, content, created_at, updated_at FROM domains WHERE deleted = 0 ORDER BY domain")
	if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	entries := make([]dbEntry, 0)
	for rows.Next() {
		var e dbEntry
		if err := rows.Scan(&e.ID, &e.Domain, &e.Port, &e.Content, &e.CreatedAt, &e.UpdatedAt); err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, entries)
}

func (a *App) handleAddDomainAction(w http.ResponseWriter, req DomainRequest) {
	domain, err := normalizeDomain(req.Domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validatePort(req.Port); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filePath := a.domainFilePath(domain)
	caddyfileContent := fmt.Sprintf("%s {\n\treverse_proxy localhost:%d\n}\n", domain, req.Port)

	tx, err := a.DB.Begin()
	if err != nil {
		http.Error(w, "Failed to begin database transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec("INSERT INTO domains (domain, port, content, created_at, updated_at, deleted) VALUES (?, ?, ?, ?, ?, ?)", domain, req.Port, caddyfileContent, now, now, 0)
	if err != nil {
		status := http.StatusInternalServerError
		if isUniqueConstraintError(err) {
			status = http.StatusConflict
		}
		http.Error(w, "Failed to save to database: "+err.Error(), status)
		return
	}

	if err := os.WriteFile(filePath, []byte(caddyfileContent), 0644); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.addDomainToCaddyAPI(domain, req.Port); err != nil {
		_ = os.Remove(filePath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		_ = os.Remove(filePath)
		_ = a.removeDomainFromCaddyAPI(domain)
		http.Error(w, "Failed to commit database transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	committed = true

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Domain %s added successfully and saved to %s", domain, filePath)
}

func (a *App) handleAddCaddyfileAction(w http.ResponseWriter, req DomainRequest) {
	if strings.TrimSpace(req.Content) == "" {
		http.Error(w, "content field is required for add-caddyfile action", http.StatusBadRequest)
		return
	}

	domain, port, err := parseCaddyfileDomainAndPort(req.Content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filename := a.domainFilePath(domain)
	tx, err := a.DB.Begin()
	if err != nil {
		http.Error(w, "Failed to begin database transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec("INSERT INTO domains (domain, port, content, created_at, updated_at, deleted) VALUES (?, ?, ?, ?, ?, ?)", domain, port, req.Content, now, now, 0)
	if err != nil {
		status := http.StatusInternalServerError
		if isUniqueConstraintError(err) {
			status = http.StatusConflict
		}
		http.Error(w, "Failed to save to database: "+err.Error(), status)
		return
	}

	if err := os.WriteFile(filename, []byte(req.Content), 0644); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := CaddyConfigUpdate(a.CaddyfilePath); err != nil {
		_ = os.Remove(filename)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		_ = os.Remove(filename)
		http.Error(w, "Failed to commit database transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	committed = true

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Caddyfile content added successfully for domain %s (port %d) and saved to %s", domain, port, filename)
}

func (a *App) handleDeleteDomainAction(w http.ResponseWriter, req DomainRequest) {
	domain, err := normalizeDomain(req.Domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var port int
	err = a.DB.QueryRow("SELECT port FROM domains WHERE domain = ? AND deleted = 0", domain).Scan(&port)
	if err == sql.ErrNoRows {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filePath := a.domainFilePath(domain)
	fileContent, fileErr := os.ReadFile(filePath)
	fileExisted := fileErr == nil
	if fileErr != nil && !os.IsNotExist(fileErr) {
		http.Error(w, "Failed to read domain file: "+fileErr.Error(), http.StatusInternalServerError)
		return
	}

	tx, err := a.DB.Begin()
	if err != nil {
		http.Error(w, "Failed to begin database transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec("UPDATE domains SET deleted = 1, updated_at = ? WHERE domain = ?", now, domain)
	if err != nil {
		http.Error(w, "Failed to delete from database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		http.Error(w, "Failed to remove file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.removeDomainFromCaddyAPI(domain); err != nil {
		restoreFile(filePath, fileContent, fileExisted)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		restoreFile(filePath, fileContent, fileExisted)
		_ = a.addDomainToCaddyAPI(domain, port)
		http.Error(w, "Failed to commit database transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	committed = true

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Domain %s deleted successfully", domain)
}

func (a *App) handleListDomainsAction(w http.ResponseWriter) {
	rows, err := a.DB.Query("SELECT domain, port FROM domains WHERE deleted = 0 ORDER BY domain")
	if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	entries := make([]domainEntry, 0)
	for rows.Next() {
		var e domainEntry
		if err := rows.Scan(&e.Domain, &e.Port); err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, entries)
}

func (a *App) handleListCaddyDomainsAction(w http.ResponseWriter) {
	routes, err := a.getCaddyRoutes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entries := make([]domainEntry, 0, len(routes))
	for _, route := range routes {
		entries = append(entries, caddyDomainEntries(route)...)
	}

	writeJSON(w, entries)
}

func (a *App) addDomainToCaddyAPI(domain string, port int) error {
	caddyRoute := map[string]interface{}{
		"match": []map[string]interface{}{
			{"host": []string{domain}},
		},
		"handle": []map[string]interface{}{
			{
				"handler": "reverse_proxy",
				"upstreams": []map[string]string{
					{"dial": fmt.Sprintf("localhost:%d", port)},
				},
			},
		},
		"terminal": true,
	}

	jsonPayload, err := json.Marshal(caddyRoute)
	if err != nil {
		return fmt.Errorf("failed to encode Caddy route: %v", err)
	}

	resp, err := a.HTTPClient.Post(a.CaddyAPIURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("Caddy API update failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Caddy API update failed: status %d", resp.StatusCode)
	}

	return nil
}

func (a *App) removeDomainFromCaddyAPI(domain string) error {
	routes, err := a.getCaddyRoutes()
	if err != nil {
		return err
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

	req, err := http.NewRequest(http.MethodPut, a.CaddyAPIURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create Caddy config request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update Caddy config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Caddy config update failed: status %d", resp.StatusCode)
	}

	return nil
}

func (a *App) getCaddyRoutes() ([]map[string]interface{}, error) {
	resp, err := a.HTTPClient.Get(a.CaddyAPIURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get Caddy config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to get Caddy config: status %d", resp.StatusCode)
	}

	var routes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return nil, fmt.Errorf("failed to decode Caddy config: %v", err)
	}
	return routes, nil
}

func (a *App) domainFilePath(domain string) string {
	return filepath.Join(a.CaddyConfigDir, domain+".caddy")
}

func parseCaddyfileDomainAndPort(content string) (string, int, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return "", 0, fmt.Errorf("content is empty")
	}

	fields := strings.Fields(strings.TrimSpace(lines[0]))
	if len(fields) == 0 {
		return "", 0, fmt.Errorf("could not extract domain from content")
	}

	domain, err := normalizeDomain(fields[0])
	if err != nil {
		return "", 0, err
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "reverse_proxy") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		port, ok := portFromAddress(parts[1])
		if ok {
			return domain, port, nil
		}
	}

	return "", 0, fmt.Errorf("could not extract port from reverse_proxy line")
}

func normalizeDomain(domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return "", fmt.Errorf("domain field is required")
	}
	if strings.ContainsAny(domain, `/\`) || strings.Contains(domain, "..") {
		return "", fmt.Errorf("invalid domain")
	}
	if len(domain) > 253 {
		return "", fmt.Errorf("domain is too long")
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("domain must include at least one dot")
	}
	for _, label := range labels {
		if !domainLabelPattern.MatchString(label) {
			return "", fmt.Errorf("invalid domain")
		}
	}

	return domain, nil
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func portFromAddress(addr string) (int, bool) {
	colonIdx := strings.LastIndex(addr, ":")
	if colonIdx == -1 || colonIdx == len(addr)-1 {
		return 0, false
	}

	port, err := strconv.Atoi(addr[colonIdx+1:])
	if err != nil || validatePort(port) != nil {
		return 0, false
	}
	return port, true
}

func caddyDomainEntries(route map[string]interface{}) []domainEntry {
	hosts := routeHosts(route)
	port := routePort(route)
	if len(hosts) == 0 {
		return nil
	}

	entries := make([]domainEntry, 0, len(hosts))
	for _, host := range hosts {
		entries = append(entries, domainEntry{Domain: host, Port: port})
	}
	return entries
}

func routeHosts(route map[string]interface{}) []string {
	match, ok := route["match"].([]interface{})
	if !ok {
		return nil
	}

	var hosts []string
	for _, item := range match {
		matcher, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		rawHosts, ok := matcher["host"].([]interface{})
		if !ok {
			continue
		}

		for _, rawHost := range rawHosts {
			host, ok := rawHost.(string)
			if ok {
				hosts = append(hosts, host)
			}
		}
	}
	return hosts
}

func routePort(route map[string]interface{}) int {
	if port := routePortFromHandles(route["handle"]); port != 0 {
		return port
	}

	return 0
}

func routePortFromHandles(rawHandles interface{}) int {
	handles, ok := rawHandles.([]interface{})
	if !ok {
		return 0
	}

	for _, item := range handles {
		handle, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		switch handle["handler"] {
		case "reverse_proxy":
			if port := reverseProxyPort(handle); port != 0 {
				return port
			}
		case "subroute":
			routes, ok := handle["routes"].([]interface{})
			if !ok {
				continue
			}
			for _, rawRoute := range routes {
				route, ok := rawRoute.(map[string]interface{})
				if !ok {
					continue
				}
				if port := routePort(route); port != 0 {
					return port
				}
			}
		}
	}

	return 0
}

func reverseProxyPort(handle map[string]interface{}) int {
	upstreams, ok := handle["upstreams"].([]interface{})
	if !ok {
		return 0
	}

	for _, rawUpstream := range upstreams {
		upstream, ok := rawUpstream.(map[string]interface{})
		if !ok {
			continue
		}

		dial, ok := upstream["dial"].(string)
		if ok {
			if port, ok := portFromAddress(dial); ok {
				return port
			}
		}
	}

	return 0
}

func routeMatchesDomain(route map[string]interface{}, domain string) bool {
	for _, host := range routeHosts(route) {
		if host == domain {
			return true
		}
	}
	return false
}

func restoreFile(path string, content []byte, existed bool) {
	if existed {
		_ = os.WriteFile(path, content, 0644)
	}
}

func isUniqueConstraintError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestE2E_AddDeleteList(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "sp-caddy-manager-e2e")

	cmd := exec.Command("go", "build", "-o", binPath, "../")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	buildOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(buildOutput))
	}

	caddyDir := filepath.Join(tmp, "caddy")
	if err := os.Mkdir(caddyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(tmp, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeCaddy := filepath.Join(binDir, "caddy")
	if err := os.WriteFile(fakeCaddy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	caddyRoutes := make([]map[string]interface{}, 0)
	caddyMu := make(chan struct{}, 1)
	caddyMu <- struct{}{}

	caddyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const routesPath = "/config/apps/http/servers/srv0/routes"
		if r.URL.Path != routesPath && !strings.HasPrefix(r.URL.Path, routesPath+"/") {
			http.NotFound(w, r)
			return
		}

		<-caddyMu
		defer func() { caddyMu <- struct{}{} }()

		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(caddyRoutes)
		case http.MethodPost:
			var route map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			caddyRoutes = append(caddyRoutes, route)
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			var routes []map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&routes); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			caddyRoutes = routes
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			idxStr := strings.TrimPrefix(r.URL.Path, routesPath+"/")
			idx, err := strconv.Atoi(idxStr)
			if err != nil || idx < 0 || idx >= len(caddyRoutes) {
				http.NotFound(w, r)
				return
			}
			caddyRoutes = append(caddyRoutes[:idx], caddyRoutes[idx+1:]...)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer caddyServer.Close()

	dbPath := filepath.Join(tmp, "domains.sqlite")
	keyDir := filepath.Join(tmp, "keys")
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keyDir, "test.key"), []byte("test-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	appCmd := exec.Command(binPath)
	appCmd.Dir = ".."
	appCmd.Env = append(os.Environ(),
		"PORT=0",
		"DB_PATH="+dbPath,
		"CADDY_CONFIG_DIR="+caddyDir,
		"CADDY_API_URL="+caddyServer.URL+"/config/apps/http/servers/srv0/routes",
		"API_KEY_DIR="+keyDir,
		"VERIFY_DOMAIN_SSL=false",
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	stdout, err := appCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	appCmd.Stderr = appCmd.Stdout

	if err := appCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = appCmd.Process.Kill()
		appCmd.Wait()
	}()

	serverURL, err := waitForServer(t, stdout)
	if err != nil {
		t.Fatal(err)
	}

	var initial []map[string]interface{}
	postJSONAction(t, serverURL, map[string]interface{}{"action": "list-db"}, &initial)

	resp := postJSON(t, serverURL+"/manage-domain", map[string]interface{}{
		"action": "add",
		"domain": "../../evil.test",
		"port":   8081,
	})
	assertStatus(t, resp, http.StatusBadRequest)
	if _, err := os.Stat(filepath.Join(tmp, "evil.test.caddy")); err == nil {
		t.Fatalf("path-like domain escaped caddy config directory")
	}

	resp = postJSON(t, serverURL+"/manage-domain", map[string]interface{}{
		"action": "add",
		"domain": "example.test",
		"port":   8081,
	})
	assertStatusOK(t, resp)

	caddyFile := filepath.Join(caddyDir, "example.test.caddy")
	if _, err := os.Stat(caddyFile); err != nil {
		t.Fatalf("expected caddy file created: %v", err)
	}

	var listAfterAdd []struct {
		Domain string `json:"domain"`
		Port   int    `json:"port"`
	}
	postJSONAction(t, serverURL, map[string]interface{}{"action": "list-db"}, &listAfterAdd)
	if len(listAfterAdd) != 1 || listAfterAdd[0].Domain != "example.test" || listAfterAdd[0].Port != 8081 {
		t.Fatalf("unexpected domain list after add: %#v", listAfterAdd)
	}

	var caddyListAfterAdd []struct {
		Domain string `json:"domain"`
		Port   int    `json:"port"`
	}
	postJSONAction(t, serverURL, map[string]interface{}{"action": "list-caddy"}, &caddyListAfterAdd)
	if len(caddyListAfterAdd) != 1 || caddyListAfterAdd[0].Domain != "example.test" || caddyListAfterAdd[0].Port != 8081 {
		t.Fatalf("unexpected caddy domain list after add: %#v", caddyListAfterAdd)
	}

	resp = postJSON(t, serverURL+"/manage-domain", map[string]interface{}{
		"action": "add",
		"domain": "example.test",
		"port":   8081,
	})
	assertStatus(t, resp, http.StatusConflict)
	postJSONAction(t, serverURL, map[string]interface{}{"action": "list-caddy"}, &caddyListAfterAdd)
	if len(caddyListAfterAdd) != 1 {
		t.Fatalf("duplicate add changed caddy routes: %#v", caddyListAfterAdd)
	}

	resp = postJSON(t, serverURL+"/manage-domain", map[string]interface{}{
		"action": "delete",
		"domain": "example.test",
	})
	assertStatusOK(t, resp)

	if _, err := os.Stat(caddyFile); err == nil {
		t.Fatalf("expected caddy file to be removed")
	}

	var finalList []map[string]interface{}
	postJSONAction(t, serverURL, map[string]interface{}{"action": "list-db"}, &finalList)

	var finalCaddyList []map[string]interface{}
	postJSONAction(t, serverURL, map[string]interface{}{"action": "list-caddy"}, &finalCaddyList)
	if len(finalCaddyList) != 0 {
		t.Fatalf("expected no caddy entries after delete, got %#v", finalCaddyList)
	}
}

func waitForServer(t *testing.T, stdout io.Reader) (string, error) {
	t.Helper()

	scanner := bufio.NewScanner(stdout)
	start := time.Now()
	re := regexp.MustCompile(`Server starting on :([0-9]+)`)

	for time.Since(start) < 10*time.Second {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			continue
		}
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) == 2 {
			return "http://127.0.0.1:" + matches[1], nil
		}
	}

	return "", fmt.Errorf("timeout waiting for server startup")
}

func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func postJSONAction(t *testing.T, serverURL string, body interface{}, target any) {
	t.Helper()
	resp := postJSON(t, serverURL+"/manage-domain", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /manage-domain returned %d: %s", resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func assertStatusOK(t *testing.T, resp *http.Response) {
	t.Helper()
	assertStatus(t, resp, http.StatusOK)
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d: %s", want, resp.StatusCode, string(body))
	}
}

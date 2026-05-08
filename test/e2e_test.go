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
	caddyRoutes := make([]map[string]interface{}, 0)
	caddyMu := make(chan struct{}, 1)
	caddyMu <- struct{}{}

	caddyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config/apps/http/servers/srv0/routes" {
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
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer caddyServer.Close()

	dbPath := filepath.Join(tmp, "domains.sqlite")
	appCmd := exec.Command(binPath)
	appCmd.Dir = ".."
	appCmd.Env = append(os.Environ(),
		"PORT=0",
		"DB_PATH="+dbPath,
		"CADDY_CONFIG_DIR="+caddyDir,
		"CADDY_API_URL="+caddyServer.URL+"/config/apps/http/servers/srv0/routes",
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

func assertGetJSON(t *testing.T, url string, target any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s returned %d: %s", url, resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}
}

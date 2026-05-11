package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sp-caddy-manager/functions"

	_ "github.com/mattn/go-sqlite3"
)

var version = "dev"

func main() {
	// Check for --version flag
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}

	functions.LoadDotEnv(".env")

	port := functions.GetPortFromEnv()
	caddyConfigDir := functions.GetCaddyConfigDir()
	caddyfilePath := functions.GetCaddyfilePath()

	cmd := exec.Command("which", "caddy")
	out, err := cmd.Output()
	if err != nil {
		log.Println("WARNING: caddy command not found in PATH")
	} else {
		log.Printf("INFO: caddy command is available at: %s", strings.TrimSpace(string(out)))
	}

	if err := os.MkdirAll(caddyConfigDir, 0744); err != nil {
		log.Fatalf("Failed to create caddy config directory: %v", err)
	}
	log.Printf("INFO: Caddy config directory ensured: %s", caddyConfigDir)

	emptyFileName := filepath.Join(caddyConfigDir, "empty.caddy")
	if _, err := os.Stat(emptyFileName); os.IsNotExist(err) {
		if err := os.WriteFile(emptyFileName, []byte("\n"), 0744); err != nil {
			log.Fatalf("Failed to create empty.caddy file: %v", err)
		}
		log.Printf("INFO: Created empty.caddy file at %s", emptyFileName)
	}

	checkCaddyfileImport(caddyfilePath, caddyConfigDir)
	validateCaddyfile(caddyfilePath)

	db, err := sql.Open("sqlite3", functions.GetDBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	app := functions.NewApp(
		db,
		caddyConfigDir,
		functions.GetCaddyAPIURL(),
		caddyfilePath,
	)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS domains (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT,
		port INTEGER,
		content TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		deleted INTEGER DEFAULT 0,
		ssl INTEGER DEFAULT 0
	)`)
	if err != nil {
		log.Fatal(err)
	}

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	actualPort := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("Server starting on :%d...\n", actualPort)
	log.Fatal(http.Serve(listener, newRouter(app)))
}

func newRouter(app *functions.App) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/manage-domain", app.HandleManageDomain)
	mux.Handle("/", http.FileServer(http.Dir("./html")))
	return mux
}

func checkCaddyfileImport(caddyfilePath, caddyConfigDir string) {
	expectedImport := "import " + filepath.Join(caddyConfigDir, "*.caddy")

	data, err := os.ReadFile(caddyfilePath)
	if err != nil {
		log.Printf("WARNING: Could not read Caddyfile %s: %v", caddyfilePath, err)
		log.Printf("WARNING: Caddyfile should include: %s", expectedImport)
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == expectedImport {
			log.Printf("INFO: Caddyfile import found: %s", expectedImport)
			return
		}
	}

	log.Printf("WARNING: Caddyfile %s is missing required import: %s", caddyfilePath, expectedImport)
}

func validateCaddyfile(caddyfilePath string) {
	cmd := exec.Command("caddy", "validate", "--config", caddyfilePath, "--adapter", "caddyfile")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("WARNING: Caddyfile validation failed for %s: %v", caddyfilePath, err)
		if output := strings.TrimSpace(string(out)); output != "" {
			log.Printf("WARNING: caddy validate output: %s", output)
		}
		return
	}
	log.Printf("INFO: Caddyfile validation passed: %s", caddyfilePath)
}

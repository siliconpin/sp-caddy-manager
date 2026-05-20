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
	functions.LoadDotEnv(".env")

	if handleCLI(os.Args[1:]) {
		return
	}
	devMode := isDevMode(os.Args[1:])

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
	log.Fatal(http.Serve(listener, newRouter(app, devMode)))
}

func handleCLI(args []string) bool {
	if len(args) == 0 {
		return false
	}

	switch args[0] {
	case "-v", "--version", "version":
		fmt.Println(version)
		return true
	case "key":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: sp-caddy-manager key <add|list|delete> [label]")
			os.Exit(2)
		}
		handleKeyCLI(args[1:])
		return true
	default:
		return false
	}
}

func handleKeyCLI(args []string) {
	store := functions.NewAPIKeyStore(functions.GetAPIKeyDir())

	switch args[0] {
	case "add":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: sp-caddy-manager key add <label>")
			os.Exit(2)
		}
		key, path, err := store.Add(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to add key:", err)
			os.Exit(1)
		}
		fmt.Printf("label: %s\n", args[1])
		fmt.Printf("key: %s\n", key)
		fmt.Printf("file: %s\n", path)
	case "list":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: sp-caddy-manager key list")
			os.Exit(2)
		}
		labels, err := store.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to list keys:", err)
			os.Exit(1)
		}
		for _, label := range labels {
			fmt.Println(label)
		}
	case "delete":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: sp-caddy-manager key delete <label>")
			os.Exit(2)
		}
		if err := store.Delete(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, "failed to delete key:", err)
			os.Exit(1)
		}
		fmt.Printf("deleted: %s\n", args[1])
	default:
		fmt.Fprintln(os.Stderr, "usage: sp-caddy-manager key <add|list|delete> [label]")
		os.Exit(2)
	}
}

func isDevMode(args []string) bool {
	return len(args) == 1 && args[0] == "dev"
}

func newRouter(app *functions.App, serveHTML bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", app.HandleAuth)
	mux.HandleFunc("/manage-domain", app.HandleManageDomain)
	if serveHTML {
		mux.Handle("/", http.FileServer(http.Dir("./html")))
	}
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

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

func main() {
	functions.LoadDotEnv(".env")

	port := functions.GetPortFromEnv()
	caddyConfigDir := functions.GetCaddyConfigDir()

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
		if err := os.WriteFile(emptyFileName, []byte(""), 0744); err != nil {
			log.Fatalf("Failed to create empty.caddy file: %v", err)
		}
		log.Printf("INFO: Created empty.caddy file at %s", emptyFileName)
	}

	db, err := sql.Open("sqlite3", functions.GetDBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	app := functions.NewApp(
		db,
		caddyConfigDir,
		functions.GetCaddyAPIURL(),
		functions.GetCaddyfilePath(),
	)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS domains (
		domain TEXT PRIMARY KEY,
		port INTEGER
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

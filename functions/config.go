package functions

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func GetEnvDefault(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

func GetDBPath() string {
	return GetEnvDefault("DB_PATH", "./domains.sqlite")
}

func GetCaddyConfigDir() string {
	return GetEnvDefault("CADDY_CONFIG_DIR", "/etc/caddy/conf.d")
}

func GetCaddyAPIURL() string {
	return GetEnvDefault("CADDY_API_URL", "http://localhost:2019/config/apps/http/servers/srv0/routes")
}

func GetCaddyfilePath() string {
	return GetEnvDefault("CADDYFILE_PATH", "/etc/caddy/Caddyfile")
}

func GetAPIKeyDir() string {
	if val := os.Getenv("API_KEY_DIR"); val != "" {
		return val
	}
	exePath, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exePath), "keys")
	}
	return "./keys"
}

func LoadDotEnv(path string) {
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
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		os.Setenv(key, strings.Trim(value, `"`))
	}
}

func GetPortFromEnv() int {
	portStr := os.Getenv("PORT")
	if portStr == "" {
		return 1011
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic("invalid PORT value " + portStr)
	}
	return port
}

func GetBackendHost() string {
	return GetEnvDefault("BACKEND_HOST", "0.0.0.0")
}

func GetVerifyDomainSSL() bool {
	value := strings.ToLower(strings.TrimSpace(GetEnvDefault("VERIFY_DOMAIN_SSL", "true")))
	return value != "false" && value != "0" && value != "no"
}

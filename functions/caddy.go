package functions

import (
	"fmt"
	"os/exec"
)

func CaddyConfigUpdate(configPath string) error {
	// Validate Caddy config
	cmd := exec.Command("caddy", "validate", "--config", configPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("caddy config validation failed: %v", err)
	}

	// Reload Caddy
	cmd = exec.Command("caddy", "reload", "--config", configPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("caddy reload failed: %v", err)
	}

	return nil
}

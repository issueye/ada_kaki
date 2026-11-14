// Credentials: the API key lives in the environment variable the settings
// name, never in config.json or the UI (docs/AI-功能规划与设计.md §F5).
package ai

import (
	"fmt"
	"os"
)

// KeyFromEnv resolves the key for a provider. A missing value is an error
// naming the variable, so the UI can say what to set.
func KeyFromEnv(env string) (string, error) {
	if env == "" {
		return "", fmt.Errorf("%w: no environment variable configured", ErrNoAPIKey)
	}
	key := os.Getenv(env)
	if key == "" {
		return "", fmt.Errorf("%w: environment variable %s is empty", ErrNoAPIKey, env)
	}
	return key, nil
}

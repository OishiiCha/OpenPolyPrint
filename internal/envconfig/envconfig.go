package envconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadFile reads a .env file and sets environment variables for any
// keys that are not already set. Existing environment variables take
// precedence — the .env file only provides defaults.
//
// Lines starting with # are comments. Blank lines are ignored.
// Values may be quoted with single or double quotes; surrounding quotes
// are stripped. Inline comments after values are not supported.
func LoadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env is optional
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first '='
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		// Expand $VAR and ${VAR} references in the value
		val = expandVars(val)

		// Don't override existing env vars
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}

	return scanner.Err()
}

// LoadDir loads .env from the given directory (if it exists).
func LoadDir(dir string) error {
	return LoadFile(filepath.Join(dir, ".env"))
}

// expandVars replaces $VAR and ${VAR} with the corresponding environment
// variable value. Unknown variables are left as-is.
func expandVars(s string) string {
	return os.Expand(s, func(name string) string {
		return os.Getenv(name)
	})
}

// Get returns the environment variable value, or the fallback if not set.
func Get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// GetBool returns the environment variable as a boolean, or the fallback
// if not set. Recognizes: true/false, 1/0, yes/no, on/off.
func GetBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return fallback
}

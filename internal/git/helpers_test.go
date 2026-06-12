package git

import (
	"os"
	"path/filepath"
)

// writeFile is a small test helper that writes content to name under
// dir with 0o644 permissions.
func writeFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

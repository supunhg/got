package store

import "io/fs"

// fsReadFile reads a file from the embedded migrations FS. Test-only
// helper used by store_test.go to assert on the migration body without
// having to write the file to disk first.
func fsReadFile(name string) (string, error) {
	data, err := fs.ReadFile(migrationsFS, name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

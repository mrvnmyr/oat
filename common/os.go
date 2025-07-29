package common

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	HomeDir = ""
)

func init() {
	var err error
	HomeDir, err = os.UserHomeDir()
	Check(err)
}

// ReadFileOrStdin reads from the given path, or from stdin if path == "-"
func ReadFileOrStdin(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// WriteFileOrStd writes to the given path, or to stdout if path == "+", or to
// stderr if path == "-"
func WriteFileOrStd(path string, data []byte, perm os.FileMode) error {
	switch path {
	case "+":
		_, err := os.Stdout.Write(data)
		return err
	case "-":
		_, err := os.Stderr.Write(data)
		return err
	}
	return os.WriteFile(path, data, perm)
}

// ExpandHome expands ~ to the user's home directory in a given path
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		// Replace only leading ~ or ~/ (not mid-path ~)
		if path == "~" {
			return HomeDir
		} else if strings.HasPrefix(path, "~/") {
			return filepath.Join(HomeDir, path[2:])
		}
	}
	return path
}

package common

import (
	"io"
	"os"
)

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

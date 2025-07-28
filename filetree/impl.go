package filetree

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mrvnmyr/oat/common"
	"gopkg.in/yaml.v3"
)

var (
	LLM bool = false

	// List of path globs to ignore, e.g. ".git/", "editorconfig/*.go", "foo/**/*.go"
	IgnoredGlobs []string = []string{
		".git/",
		".task/",
		"node_modules/",
	}

	// List of path globs to include only
	AllowedGlobs []string = []string{}

	// SkipBinaryFiles controls whether binary files are skipped when serializing
	SkipBinaryFiles bool = true
)

// shouldIgnore returns true if relPath matches any glob.
func shouldIgnore(relPath string) bool {
	relPath = strings.TrimPrefix(relPath, "./")
	relPathSlash := relPath
	if !strings.HasSuffix(relPathSlash, "/") && isDirGlobMatch(relPath) {
		relPathSlash += "/"
	}

	match := func(glob string) bool {
		if strings.HasSuffix(glob, "/") {
			if strings.HasPrefix(relPathSlash, glob) {
				return true
			}
		}
		ok, err := path.Match(glob, relPath)
		if err == nil && ok {
			return true
		}
		return false
	}

	for _, glob := range IgnoredGlobs {
		if match(glob) {
			return true
		}
	}
	return false
}

// shouldAllow returns true if relPath matches any glob in AllowedGlobs, or if the list is empty.
func shouldAllow(relPath string) bool {
	if len(AllowedGlobs) == 0 {
		return true
	}
	relPath = strings.TrimPrefix(relPath, "./")
	relPathSlash := relPath
	if !strings.HasSuffix(relPathSlash, "/") && isDirGlobMatch(relPath) {
		relPathSlash += "/"
	}

	match := func(glob string) bool {
		if strings.HasSuffix(glob, "/") {
			if strings.HasPrefix(relPathSlash, glob) {
				return true
			}
		}
		ok, err := path.Match(glob, relPath)
		if err == nil && ok {
			return true
		}
		return false
	}

	for _, glob := range AllowedGlobs {
		if match(glob) {
			return true
		}
	}
	return false
}

// Heuristic to determine if a pattern is intended as a dir (for trailing slash globs)
func isDirGlobMatch(relPath string) bool {
	return strings.HasSuffix(relPath, "/")
}

// Entry represents a file (directories are no longer represented).
type Entry struct {
	Perm    string `yaml:"perm"`
	Content string `yaml:"content,omitempty"`
}

// isLikelyBinaryFile returns true if the file at 'path' appears binary.
func isLikelyBinaryFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	const sniffLen = 8000
	buf := make([]byte, sniffLen)
	n, err := f.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return false, err
	}
	buf = buf[:n]

	if n == 0 {
		return false, nil // empty file is not binary
	}
	if !utf8.Valid(buf) {
		return true, nil
	}
	for _, b := range buf {
		if b == 0 {
			return true, nil
		}
	}
	return false, nil
}

func globToRegexp(glob string) string {
	var rx strings.Builder
	rx.WriteString("^")
	for i := 0; i < len(glob); {
		if strings.HasPrefix(glob[i:], "**/") {
			rx.WriteString("(?:.*/)?")
			i += 3
		} else if strings.HasPrefix(glob[i:], "**") {
			rx.WriteString(".*")
			i += 2
		} else {
			switch glob[i] {
			case '*':
				rx.WriteString("[^/]*")
			case '?':
				rx.WriteString("[^/]")
			case '.', '+', '(', ')', '$', '^', '|', '{', '}', '[', ']', '\\':
				rx.WriteString("\\" + string(glob[i]))
			default:
				rx.WriteByte(glob[i])
			}
			i++
		}
	}
	rx.WriteString("$")
	return rx.String()
}

func matchIncludeOnly(relPath string, includeOnly []string) bool {
	relPath = strings.TrimPrefix(relPath, "./")
	relPathSlash := relPath
	if !strings.HasSuffix(relPathSlash, "/") && isDirGlobMatch(relPath) {
		relPathSlash += "/"
	}
	if len(includeOnly) == 0 {
		return true
	}
	for _, pat := range includeOnly {
		pat = strings.TrimPrefix(pat, "./")
		if strings.HasSuffix(pat, "/") {
			if strings.HasPrefix(relPathSlash, pat) {
				return true
			}
			continue
		}
		if strings.Contains(pat, "**") {
			rx := globToRegexp(pat)
			if ok, _ := regexp.MatchString(rx, relPath); ok {
				return true
			}
			continue
		}
		if strings.ContainsAny(pat, "*?[") {
			ok, err := path.Match(pat, relPath)
			if err == nil && ok {
				return true
			}
			continue
		}
		if relPath == pat {
			return true
		}
	}
	return false
}

// DirTreeToYAML walks 'srcRoot' and outputs a map[path]Entry as YAML at yamlPath.
// Only files are output; directories are omitted.
func DirTreeToYAML(srcRoot, yamlPath string, includeOnly []string) error {
	tree := map[string]Entry{}
	err := filepath.Walk(srcRoot, func(pathStr string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if pathStr == srcRoot {
			return nil // skip root
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() {
			relPath, err := filepath.Rel(srcRoot, pathStr)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)
			if shouldIgnore(relPath) {
				return filepath.SkipDir
			}
			return nil
		}
		relPath, err := filepath.Rel(srcRoot, pathStr)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		if shouldIgnore(relPath) {
			return nil
		}
		if !shouldAllow(relPath) {
			return nil
		}
		if !matchIncludeOnly(relPath, includeOnly) {
			return nil
		}
		if SkipBinaryFiles {
			isBin, err := isLikelyBinaryFile(pathStr)
			if err != nil {
				return err
			}
			if isBin {
				return nil
			}
		}
		b, err := common.ReadFileOrStdin(pathStr)
		if err != nil {
			return err
		}
		entry := Entry{
			Perm:    fmt.Sprintf("%04o", info.Mode().Perm()),
			Content: string(b),
		}
		tree[relPath] = entry
		return nil
	})
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(tree)
	if err != nil {
		return err
	}

	var result []byte
	if LLM {
		result = []byte("```\n")
		result = append(result, out...)
		result = append(result, []byte("```\n\nThis is a flattened filetree represented as a YAML.\n\nTODO\n\nImplement what is required to fix this issue and output it in the same flattened filetree YAML structure as was provided before.\n\nIf files are not changed don't output them.\n")...)
	} else {
		result = out
	}

	return common.WriteFileOrStd(yamlPath, result, 0644)
}

// YAMLToDirTree reads YAML file describing a tree and creates files under destRoot.
// Directories are not created unless needed for files.
func YAMLToDirTree(yamlPath, destRoot string) error {
	data, err := common.ReadFileOrStdin(yamlPath)
	if err != nil {
		return err
	}
	tree := map[string]Entry{}
	if err := yaml.Unmarshal(data, &tree); err != nil {
		return err
	}
	for f, entry := range tree {
		full := filepath.Join(destRoot, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := common.WriteFileOrStd(full, []byte(entry.Content), 0o644); err != nil {
			return err
		}
		perm, _ := parsePerm(entry.Perm)
		if err := os.Chmod(full, perm); err != nil {
			return err
		}
	}
	return nil
}

// parsePerm parses a string like "0755" to os.FileMode
func parsePerm(s string) (os.FileMode, error) {
	var perm uint32
	_, err := fmt.Sscanf(s, "%o", &perm)
	return os.FileMode(perm), err
}

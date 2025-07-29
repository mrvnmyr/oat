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

	IgnoredGlobs []string = []string{
		".git/",
		".task/",
		"node_modules/",
	}

	AllowedGlobs []string = []string{}

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

func isDirGlobMatch(relPath string) bool {
	return strings.HasSuffix(relPath, "/")
}

type Entry struct {
	Perm    string `yaml:"perm"`
	Content string `yaml:"content"`
}

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
// 'seeksDotFiles' controls if we seek .flattenignore/.flattenallow for "no arg" mode
func DirTreeToYAML(srcRoot, yamlPath string, includeOnly []string, seeksDotFiles bool) error {
	var err error

	if seeksDotFiles && srcRoot == "" {
		srcRoot, err = findRootAndPopulateFromDotFlattenFile(srcRoot)
		common.Check(err)
	}

	tree := map[string]Entry{}
	err = filepath.Walk(srcRoot, func(pathStr string, info os.FileInfo, err error) error {
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
			if !seeksDotFiles && !shouldProcessIgnores() {
				// nothing, just don't skip
			} else {
				if shouldIgnore(relPath) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		relPath, err := filepath.Rel(srcRoot, pathStr)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		if !seeksDotFiles && !shouldProcessIgnores() {
			// skip nothing
		} else {
			if shouldIgnore(relPath) {
				return nil
			}
			if !shouldAllow(relPath) {
				return nil
			}
			if !matchIncludeOnly(relPath, includeOnly) {
				return nil
			}
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

// FlattenArgsToYAML handles flattening files/dirs passed as args, optionally without ignores.
func FlattenArgsToYAML(paths []string, yamlPath string, noIgnores bool) error {
	tree := map[string]Entry{}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	for _, root := range paths {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		isBelow, relBase := pathIsBelowCWD(absRoot, cwd)
		err = flattenArgAddWithBase(tree, root, "", noIgnores, absRoot, isBelow, relBase)
		if err != nil {
			return err
		}
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

// Helper for FlattenArgsToYAML: handles one file/dir, recursively, using absRoot/isBelowCWD info
func flattenArgAddWithBase(tree map[string]Entry, src string, prefix string, noIgnores bool, absRoot string, isBelow bool, relBase string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if info.IsDir() {
		return filepath.Walk(src, func(pathStr string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			absPath, err := filepath.Abs(pathStr)
			if err != nil {
				return err
			}
			var relPath string
			if isBelow {
				rp, err := filepath.Rel(relBase, absPath)
				if err != nil {
					return err
				}
				relPath = filepath.ToSlash(rp)
			} else {
				relPath = filepath.ToSlash(absPath)
			}
			if prefix != "" {
				relPath = path.Join(prefix, relPath)
			}
			if !noIgnores {
				if shouldIgnore(relPath) {
					return nil
				}
				if !shouldAllow(relPath) {
					return nil
				}
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
			tree[relPath] = Entry{
				Perm:    fmt.Sprintf("%04o", info.Mode().Perm()),
				Content: string(b),
			}
			return nil
		})
	} else {
		absPath, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		var relPath string
		if isBelow {
			rp, err := filepath.Rel(relBase, absPath)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(rp)
		} else {
			relPath = filepath.ToSlash(absPath)
		}
		if prefix != "" {
			relPath = path.Join(prefix, filepath.Base(src))
		}
		if !noIgnores {
			if shouldIgnore(relPath) {
				return nil
			}
			if !shouldAllow(relPath) {
				return nil
			}
		}
		if SkipBinaryFiles {
			isBin, err := isLikelyBinaryFile(src)
			if err != nil {
				return err
			}
			if isBin {
				return nil
			}
		}
		b, err := common.ReadFileOrStdin(src)
		if err != nil {
			return err
		}
		tree[relPath] = Entry{
			Perm:    fmt.Sprintf("%04o", info.Mode().Perm()),
			Content: string(b),
		}
	}
	return nil
}

// Returns (isBelowCWD, relBase)
func pathIsBelowCWD(absTarget string, cwd string) (bool, string) {
	cwdAbs := cwd
	if !filepath.IsAbs(cwdAbs) {
		cwdAbs, _ = filepath.Abs(cwd)
	}
	rel, err := filepath.Rel(cwdAbs, absTarget)
	if err != nil {
		return false, cwdAbs
	}
	if !strings.HasPrefix(rel, "..") && rel != "." {
		return true, cwdAbs
	}
	return false, cwdAbs
}

// You may want to define shouldProcessIgnores() as always true here, or remove all usage;
// it's just an example for clarity and is not required.
func shouldProcessIgnores() bool {
	return true
}

func findRootAndPopulateFromDotFlattenFile(srcRoot string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return srcRoot, err
	}

	// Walk upwards looking for either .flattenignore or .flattenallow
	dir := cwd

	fillVar := func(path string, variable *[]string) (found bool, err error) {
		if _, err := os.Stat(path); err == nil {
			lines, err := os.ReadFile(path)
			if err != nil {
				return false, fmt.Errorf("reading %s: %w", path, err)
			}
			// empty variable first
			*variable = []string{}

			{ // process the lines, handle '#' comments and '< file' to insert file contents
				var processLines func([]string, string) error

				processLines = func(lines []string, dir string) error {
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line == "" || strings.HasPrefix(line, "#") {
							continue
						}
						if strings.HasPrefix(line, "< ") {
							insertPath := strings.TrimSpace(line[2:])
							insertPath = common.ExpandHome(insertPath)
							if !filepath.IsAbs(insertPath) {
								insertPath = filepath.Join(dir, insertPath)
							}
							inserted, err := os.ReadFile(insertPath)
							if err != nil {
								return fmt.Errorf("reading inserted file %s: %w", insertPath, err)
							}
							insertedLines := strings.Split(string(inserted), "\n")
							// recurse to process included lines (may include more < ...)
							if err := processLines(insertedLines, filepath.Dir(insertPath)); err != nil {
								return err
							}
							continue
						}
						*variable = append(*variable, line)
					}
					return nil
				}

				*variable = []string{}
				if err := processLines(strings.Split(string(lines), "\n"), filepath.Dir(path)); err != nil {
					return false, err
				}
			}

			srcRoot = dir
			return true, nil
		}
		return false, nil
	}

	for {
		foundAny := false

		found, err := fillVar(filepath.Join(dir, ".flattenignore"), &IgnoredGlobs)
		common.Check(err)
		foundAny = foundAny || found

		found, err = fillVar(filepath.Join(dir, ".flattenallow"), &AllowedGlobs)
		common.Check(err)
		foundAny = foundAny || found

		if foundAny {
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return srcRoot, fmt.Errorf("no .flattenignore or .flattenallow file found while searching from %s upwards", cwd)
		}
		dir = parent
	}

	return srcRoot, nil
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

func parsePerm(s string) (os.FileMode, error) {
	var perm uint32
	_, err := fmt.Sscanf(s, "%o", &perm)
	return os.FileMode(perm), err
}

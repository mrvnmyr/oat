package filetree

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
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

// normalizeFiletreePath makes "./foo" and "foo" equivalent while preserving
// absolute paths, parent traversals, wildcards, and trailing slashes.
func normalizeFiletreePath(p string) string {
	p = filepath.ToSlash(p)
	for strings.HasPrefix(p, "./") {
		p = strings.TrimPrefix(p, "./")
	}
	if p == "." {
		return ""
	}
	return p
}

// shouldIgnore returns true if relPath matches any glob.
func shouldIgnore(relPath string) bool {
	relPath = normalizeFiletreePath(relPath)

	match := func(glob string) bool {
		glob = normalizeFiletreePath(glob)
		if strings.HasSuffix(glob, "/") && strings.HasPrefix(relPath, glob) {
			return true
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
	relPath = normalizeFiletreePath(relPath)

	match := func(glob string) bool {
		glob = normalizeFiletreePath(glob)
		if strings.HasSuffix(glob, "/") && strings.HasPrefix(relPath, glob) {
			return true
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

type Entry struct {
	Perm    string                `yaml:"perm,omitempty"`
	Content *common.LiteralString `yaml:"content,omitempty"`
	Patch   *common.LiteralString `yaml:"patch,omitempty"`
}

func newContentEntry(perm os.FileMode, b []byte) Entry {
	content := common.LiteralString(b)
	return Entry{
		Perm:    fmt.Sprintf("%04o", perm.Perm()),
		Content: &content,
	}
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
	relPath = normalizeFiletreePath(relPath)
	if len(includeOnly) == 0 {
		return true
	}
	for _, pat := range includeOnly {
		pat = normalizeFiletreePath(pat)
		if strings.HasSuffix(pat, "/") {
			if strings.HasPrefix(relPath, pat) {
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
			relPath = normalizeFiletreePath(relPath)
			if !seeksDotFiles && !shouldProcessIgnores() {
				// nothing, just don't skip
			} else {
				if shouldIgnore(relPath + "/") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		relPath, err := filepath.Rel(srcRoot, pathStr)
		if err != nil {
			return err
		}
		relPath = normalizeFiletreePath(relPath)
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
		tree[relPath] = newContentEntry(info.Mode(), b)
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
		common.Debugf("filetree flatten: root=%q abs=%q below-cwd=%t rel-base=%q\n",
			root, absRoot, isBelow, relBase)
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
		result = append(result, []byte("```\n\nThis is a flattened filetree represented as a YAML.\n\nTODO\n\nImplement what is required to fix this issue and output it in the same flattened filetree YAML structure as was provided before.\n\nDo not invent a 'patch' field. You are not allowed to return patches, but must always return file contents in full!\n\nIf files are not changed don't output them.\n")...)
	} else {
		result = out
	}
	return common.WriteFileOrStd(yamlPath, result, 0644)
}

// Helper for FlattenArgsToYAML: handles one file/dir, recursively, using absRoot/isBelowCWD info
func flattenArgAddWithBase(tree map[string]Entry, src string, prefix string, noIgnores bool, absRoot string, isBelow bool, relBase string) error {
	common.Debugf("Flatten: %s\n", src)
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
				relPath = normalizeFiletreePath(rp)
			} else {
				relPath = normalizeFiletreePath(absPath)
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
			tree[relPath] = newContentEntry(info.Mode(), b)
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
			relPath = normalizeFiletreePath(rp)
		} else {
			relPath = normalizeFiletreePath(absPath)
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
		tree[relPath] = newContentEntry(info.Mode(), b)
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
	if rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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

	seen := map[string]string{}
	for f, entry := range tree {
		normalized := normalizeFiletreePath(f)
		if normalized == "" {
			return fmt.Errorf("invalid empty filetree path %q", f)
		}
		if previous, ok := seen[normalized]; ok {
			return fmt.Errorf("filetree paths %q and %q resolve to the same path %q", previous, f, normalized)
		}
		seen[normalized] = f

		full := filepath.Join(destRoot, filepath.FromSlash(normalized))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}

		switch {
		case entry.Content != nil && entry.Patch != nil:
			return fmt.Errorf("%s: entry cannot contain both content and patch", f)

		case entry.Patch != nil:
			common.Debugf("filetree expand: patch %q -> %q\n", f, full)
			original, err := os.ReadFile(full)
			if err != nil {
				return fmt.Errorf("reading patch target %s: %w", full, err)
			}
			result, err := applyUnifiedPatch(original, string(*entry.Patch))
			if err != nil {
				return fmt.Errorf("applying patch to %s: %w", full, err)
			}
			if err := os.WriteFile(full, result, 0o644); err != nil {
				return err
			}

		case entry.Content != nil:
			common.Debugf("filetree expand: content %q -> %q\n", f, full)
			if err := common.WriteFileOrStd(full, []byte(*entry.Content), 0o644); err != nil {
				return err
			}

		default:
			return fmt.Errorf("%s: entry must contain either content or patch", f)
		}

		if entry.Perm != "" {
			perm, err := parsePerm(entry.Perm)
			if err != nil {
				return fmt.Errorf("%s: invalid perm %q: %w", f, entry.Perm, err)
			}
			if err := os.Chmod(full, perm); err != nil {
				return err
			}
		}
	}
	return nil
}

type patchLine struct {
	op        byte
	text      string
	noNewline bool
}

type patchHunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	lines    []patchLine
}

type textLine struct {
	text    string
	newline bool
}

var reUnifiedHunk = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

func parseUnifiedPatch(patchText string) ([]patchHunk, error) {
	lines := strings.Split(strings.ReplaceAll(patchText, "\r\n", "\n"), "\n")
	hunks := make([]patchHunk, 0)

	for i := 0; i < len(lines); {
		matches := reUnifiedHunk.FindStringSubmatch(lines[i])
		if len(matches) == 0 {
			i++
			continue
		}

		oldStart, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, err
		}
		oldCount, err := parseHunkCount(matches[2])
		if err != nil {
			return nil, err
		}
		newStart, err := strconv.Atoi(matches[3])
		if err != nil {
			return nil, err
		}
		newCount, err := parseHunkCount(matches[4])
		if err != nil {
			return nil, err
		}

		hunk := patchHunk{
			oldStart: oldStart,
			oldCount: oldCount,
			newStart: newStart,
			newCount: newCount,
		}
		i++

		oldSeen := 0
		newSeen := 0
		for oldSeen < oldCount || newSeen < newCount {
			if i >= len(lines) {
				return nil, fmt.Errorf(
					"incomplete hunk -%d,%d +%d,%d",
					oldStart, oldCount, newStart, newCount,
				)
			}

			line := lines[i]
			if line == `\ No newline at end of file` {
				if len(hunk.lines) == 0 {
					return nil, fmt.Errorf("newline marker without a preceding hunk line")
				}
				hunk.lines[len(hunk.lines)-1].noNewline = true
				i++
				continue
			}
			if reUnifiedHunk.MatchString(line) {
				return nil, fmt.Errorf(
					"hunk -%d,%d +%d,%d ended before its declared line counts",
					oldStart, oldCount, newStart, newCount,
				)
			}
			if line == "" {
				return nil, fmt.Errorf(
					"unexpected empty patch line in hunk -%d,%d +%d,%d",
					oldStart, oldCount, newStart, newCount,
				)
			}

			pl := patchLine{op: line[0], text: line[1:]}
			switch pl.op {
			case ' ':
				oldSeen++
				newSeen++
			case '-':
				oldSeen++
			case '+':
				newSeen++
			default:
				return nil, fmt.Errorf("invalid unified-diff line %q", line)
			}

			if oldSeen > oldCount || newSeen > newCount {
				return nil, fmt.Errorf(
					"hunk -%d,%d +%d,%d exceeds its declared line counts",
					oldStart, oldCount, newStart, newCount,
				)
			}

			hunk.lines = append(hunk.lines, pl)
			i++
		}

		if i < len(lines) && lines[i] == `\ No newline at end of file` {
			if len(hunk.lines) == 0 {
				return nil, fmt.Errorf("newline marker without a preceding hunk line")
			}
			hunk.lines[len(hunk.lines)-1].noNewline = true
			i++
		}

		common.Debugf("filetree patch: hunk -%d,%d +%d,%d\n",
			oldStart, oldCount, newStart, newCount)
		hunks = append(hunks, hunk)
	}

	if len(hunks) == 0 {
		return nil, fmt.Errorf("patch contains no unified-diff hunks")
	}
	return hunks, nil
}

func parseHunkCount(s string) (int, error) {
	if s == "" {
		return 1, nil
	}
	return strconv.Atoi(s)
}

func hunkLineIndex(start, count int) int {
	if count == 0 {
		return start
	}
	return start - 1
}

func splitTextLines(data []byte) []textLine {
	if len(data) == 0 {
		return nil
	}

	parts := strings.SplitAfter(string(data), "\n")
	lines := make([]textLine, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if strings.HasSuffix(part, "\n") {
			lines = append(lines, textLine{
				text:    strings.TrimSuffix(part, "\n"),
				newline: true,
			})
		} else {
			lines = append(lines, textLine{text: part})
		}
	}
	return lines
}

func joinTextLines(lines []textLine) ([]byte, error) {
	var result strings.Builder
	for i, line := range lines {
		if !line.newline && i != len(lines)-1 {
			return nil, fmt.Errorf("patch produced a non-final line without a newline")
		}
		result.WriteString(line.text)
		if line.newline {
			result.WriteByte('\n')
		}
	}
	return []byte(result.String()), nil
}

func applyUnifiedPatch(original []byte, patchText string) ([]byte, error) {
	hunks, err := parseUnifiedPatch(patchText)
	if err != nil {
		return nil, err
	}

	source := splitTextLines(original)
	result := make([]textLine, 0, len(source))
	sourcePos := 0

	for hunkIndex, hunk := range hunks {
		oldIndex := hunkLineIndex(hunk.oldStart, hunk.oldCount)
		newIndex := hunkLineIndex(hunk.newStart, hunk.newCount)
		if oldIndex < sourcePos || oldIndex > len(source) {
			return nil, fmt.Errorf("hunk %d old position is out of range", hunkIndex+1)
		}

		result = append(result, source[sourcePos:oldIndex]...)
		if newIndex != len(result) {
			return nil, fmt.Errorf(
				"hunk %d target position mismatch: patch expects line index %d, got %d",
				hunkIndex+1, newIndex, len(result),
			)
		}
		sourcePos = oldIndex

		for _, pl := range hunk.lines {
			switch pl.op {
			case ' ', '-':
				if sourcePos >= len(source) {
					return nil, fmt.Errorf("hunk %d reads past end of target", hunkIndex+1)
				}
				sourceLine := source[sourcePos]
				if sourceLine.text != pl.text || sourceLine.newline == pl.noNewline {
					return nil, fmt.Errorf(
						"hunk %d does not match target at line %d",
						hunkIndex+1, sourcePos+1,
					)
				}
				if pl.op == ' ' {
					result = append(result, sourceLine)
				}
				sourcePos++

			case '+':
				result = append(result, textLine{
					text:    pl.text,
					newline: !pl.noNewline,
				})
			}
		}
	}

	result = append(result, source[sourcePos:]...)
	return joinTextLines(result)
}

func parsePerm(s string) (os.FileMode, error) {
	var perm uint32
	_, err := fmt.Sscanf(s, "%o", &perm)
	return os.FileMode(perm), err
}

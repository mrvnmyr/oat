package common

import (
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type LiteralString string

// stripTrailingWhitespaceFromLines splits a string into lines,
// trims trailing whitespace from each line, and rejoins them.
func stripTrailingWhitespaceFromLines(s string) string {
	lines := strings.Split(s, "\n")

	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}

	return strings.Join(lines, "\n")
}

func (s LiteralString) MarshalYAML() (interface{}, error) {
	// Debugf("yaml: marshaling literal string (%d bytes)\n", len(s))

	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Style: yaml.LiteralStyle,
		Tag:   "!!str",
		Value: stripTrailingWhitespaceFromLines(string(s)),
	}, nil
}

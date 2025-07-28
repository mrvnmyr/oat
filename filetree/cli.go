package filetree

import (
	"github.com/mrvnmyr/oat/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use: "filetree",
}

var cmdFlatten = &cobra.Command{
	Use:  "flatten [path-root] [include-only...]",
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		path := "."
		var rest []string

		if len(args) >= 1 {
			path = args[0]
		}
		if len(args) > 1 {
			rest = args[1:]
		}

		err := DirTreeToYAML(path, "+", rest)
		common.Check(err)
	},
}
var cmdExpand = &cobra.Command{
	Use:  "expand [input] [output-root]",
	Args: cobra.RangeArgs(0, 1),
	Run: func(cmd *cobra.Command, args []string) {
		path := "-"
		outputRoot := "."
		if len(args) >= 1 {
			path = args[0]
		}
		if len(args) >= 2 {
			outputRoot = args[1]
		}

		err := YAMLToDirTree(path, outputRoot)
		common.Check(err)
	},
}

func init() {

	Cmd.AddCommand(cmdFlatten)
	cmdFlatten.PersistentFlags().BoolVar(&SkipBinaryFiles, "skip-binary-files", true, "Skip binary files")
	cmdFlatten.PersistentFlags().BoolVar(&LLM, "llm", false, "Output in LLM prompt format")
	cmdFlatten.PersistentFlags().StringArrayVar(&IgnoredGlobs, "ignored-globs", []string{".git/", ".task/", "node_modules/"}, "IgnoredGlobs (Blocklist)")
	cmdFlatten.PersistentFlags().StringArrayVar(&AllowedGlobs, "allowed-globs", []string{}, "AllowedGlobs (Allowlist)")
	Cmd.AddCommand(cmdExpand)
}

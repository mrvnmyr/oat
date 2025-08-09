package filetree

import (
	"github.com/mrvnmyr/oat/common"
	"github.com/spf13/cobra"
)

var NoIgnores bool

var Cmd = &cobra.Command{
	Use: "filetree",
}

var cmdFlatten = &cobra.Command{
	Use:  "flatten [files-or-dirs...]",
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// expand ~ in args
		for i, _ := range args {
			args[i] = common.ExpandHome(args[i])
		}
		if len(args) == 0 {
			// Seek .flattenignore/.flattenallow as before
			err := DirTreeToYAML("", "+", []string{}, true)
			common.Check(err)
			return
		}

		// Pass args as files or directories to flatten
		err := FlattenArgsToYAML(args, "+", NoIgnores)
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
	cmdFlatten.PersistentFlags().BoolVar(&NoIgnores, "no-ignores", false, "Do not apply any ignores/allow filtering in flatten mode; only flatten the listed files/dirs")
	Cmd.AddCommand(cmdExpand)
}

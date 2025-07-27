package editorconfig

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:  "editorconfig <path-to-any-file> [optional-key]",
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 1 {
			Calc(args[0], "")
		} else {
			Calc(args[0], args[1])
		}
	},
}

func init() {
}

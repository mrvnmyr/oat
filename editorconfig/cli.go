package editorconfig

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:  "editorconfig [path-to-any-file]",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		Calc(args[0])
	},
}

func init() {
}

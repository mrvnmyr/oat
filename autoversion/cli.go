package autoversion

import (
	"fmt"
	"os"

	"github.com/mrvnmyr/oat/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:  "autoversion <path-to-file>",
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var err error

		if len(args) == 0 || len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			fmt.Printf("pass me a file, %s\n", helpText)
			os.Exit(0)
		}

		for _, arg := range args {
			err = autoversionFile(arg)
			common.Check(err)
		}
	},
}

package main

import (
	"github.com/mrvnmyr/oat/common"
	"github.com/mrvnmyr/oat/editorconfig"
	"github.com/mrvnmyr/oat/filetree"
	"github.com/spf13/cobra"
)

var cmdRoot = &cobra.Command{
	Use: "oat",
}

func init() {
	cmdRoot.SetHelpCommand(&cobra.Command{Hidden: true}) // hide help

	cmdRoot.PersistentFlags().BoolVar(&common.DebugFlag, "debug", false, "Enable debug output")
	cmdRoot.AddCommand(editorconfig.Cmd)
	cmdRoot.AddCommand(filetree.Cmd)
}

func Cli() {
	err := cmdRoot.Execute()
	common.Check(err)
}

package cmd

import (
	"github.com/JohnNeville/autorestic/internal"
	"github.com/JohnNeville/autorestic/internal/colors"
	"github.com/JohnNeville/autorestic/internal/lock"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check if everything is setup",
	Run: func(cmd *cobra.Command, args []string) {
		internal.GetConfig()
		err := lock.Lock()
		CheckErr(err)
		defer lock.Unlock()

		CheckErr(internal.CheckConfig())

		colors.Success.Println("Everything is fine.")
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

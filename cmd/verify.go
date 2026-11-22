package cmd

import (
	"github.com/JohnNeville/autorestic/internal"
	"github.com/JohnNeville/autorestic/internal/lock"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Check the integrity of the repositories a location backs up to (restic check)",
	Run: func(cmd *cobra.Command, args []string) {
		internal.GetConfig()
		err := lock.Lock()
		CheckErr(err)
		defer lock.Unlock()

		selected, err := internal.GetAllOrSelected(cmd, false)
		CheckErr(err)

		var errors []error
		for _, name := range selected {
			location, _ := internal.GetLocation(name)
			if err := location.Verify(); err != nil {
				errors = append(errors, err)
			}
		}

		CheckErr(errorsFound(errors))
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
	internal.AddFlagsToCommand(verifyCmd, false)
}

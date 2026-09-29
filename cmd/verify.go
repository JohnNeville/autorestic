package cmd

import (
	"fmt"

	"github.com/JohnNeville/autorestic/internal"
	"github.com/JohnNeville/autorestic/internal/colors"
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

		if len(errors) > 0 {
			for _, err := range errors {
				colors.Error.Printf("%s\n\n", err)
			}

			CheckErr(fmt.Errorf("%d errors were found", len(errors)))
		}
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
	internal.AddFlagsToCommand(verifyCmd, false)
}

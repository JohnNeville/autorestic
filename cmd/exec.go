package cmd

import (
	"github.com/JohnNeville/autorestic/internal"
	"github.com/JohnNeville/autorestic/internal/colors"
	"github.com/JohnNeville/autorestic/internal/lock"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute arbitrary native restic commands for given backends",
	Run: func(cmd *cobra.Command, args []string) {
		internal.GetConfig()
		err := lock.Lock()
		CheckErr(err)
		defer lock.Unlock()

		selected, err := internal.GetAllOrSelected(cmd, true)
		CheckErr(err)

		var errors []error
		for _, name := range selected {
			colors.PrimaryPrint("  Executing on \"%s\"  ", name)
			backend, _ := internal.GetBackend(name)
			err := backend.Exec(args)
			if err != nil {
				errors = append(errors, err)
			}
		}

		CheckErr(errorsFound(errors))
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
	internal.AddFlagsToCommand(execCmd, true)
}

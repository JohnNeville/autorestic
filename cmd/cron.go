package cmd

import (
	"github.com/JohnNeville/autorestic/internal"
	"github.com/JohnNeville/autorestic/internal/flags"
	"github.com/JohnNeville/autorestic/internal/lock"
	"github.com/spf13/cobra"
)

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Run cron job for automated backups",
	Long:  `Intended to be mainly triggered by an automated system like systemd or crontab. For each location checks if a cron backup is due and runs it.`,
	Run: func(cmd *cobra.Command, args []string) {
		internal.GetConfig()
		err := lock.Lock()
		CheckErr(err)
		defer lock.Unlock()

		err = internal.RunCron()
		CheckErr(err)
	},
}

func init() {
	rootCmd.AddCommand(cronCmd)
	cronCmd.Flags().BoolVar(&flags.CRON_LEAN, "lean", false, "only output information about actual backups")
}

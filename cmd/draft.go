package cmd

import (
	"github.com/spf13/cobra"
)

// draftCmd is publish --dry-run under its own name: it chooses the day's
// post, writes and reviews the text and prints what would go out.
var draftCmd = &cobra.Command{
	Use:   "draft",
	Short: "Write the day's posts and print them without publishing",
	Long: `Choose the day's post, write and review the text for each platform and print it,
with the images and their alt text, without publishing anything or writing the ledger.

The same as publish --dry-run.

Examples:
  uac-socializer draft
  uac-socializer draft --date 09-14 --platform mastodon`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cmd.Flags().Set("dry-run", "true"); err != nil {
			return err
		}
		return runPublishCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(draftCmd)
	draftCmd.Flags().Bool("dry-run", true, "Always true for draft")
	_ = draftCmd.Flags().MarkHidden("dry-run")
	draftCmd.Flags().String("date", "", "Target date in MM-DD format (default: today)")
	draftCmd.Flags().String("platform", "all", "Target platform: bluesky|mastodon|all")
	draftCmd.Flags().String("program", "", "Program: on-this-day|place-of-the-week|collection|artist-spotlight|back-then (default: the weekly rhythm)")
}

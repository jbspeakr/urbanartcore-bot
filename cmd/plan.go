package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/ledger"
	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/site"
	"github.com/jbspeakr/urbanartcore-bot/internal/app"
	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/infrastructure"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Print which post goes out on each of the coming days",
	Long: `Print the plan for the coming days: the program, the chosen post, its score and what
the score is made of, or a rest day. The plan reads the catalogue and, when
UAC_LEDGER_BUCKET is set, the ledger; it makes no model calls and publishes nothing.

Examples:
  uac-socializer plan
  uac-socializer plan --days 30 --from 12-01`,
	RunE: runPlanCommand,
}

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.Flags().Int("days", 14, "Number of days to plan")
	planCmd.Flags().String("from", "", "First day, YYYY-MM-DD or MM-DD in the current year (default: today)")
}

func runPlanCommand(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	days, _ := cmd.Flags().GetInt("days")
	fromFlag, _ := cmd.Flags().GetString("from")
	if days < 1 || days > 366 {
		return fmt.Errorf("--days must be between 1 and 366 (got %d)", days)
	}
	from, err := parsePlanStart(fromFlag, time.Now().UTC())
	if err != nil {
		return err
	}

	config, err := app.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	// The table goes to stdout; only warnings reach stderr
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       slog.LevelWarn,
		ReplaceAttr: infrastructure.SanitizeSecrets,
	})))

	catalogue, err := site.NewClient(config.SiteBaseURL)
	if err != nil {
		return fmt.Errorf("failed to initialize site client: %w", err)
	}
	var history ports.Ledger
	if config.HasLedger() {
		objectStorage, err := ledger.NewObjectStorage(ledger.Config{
			Endpoint:  config.LedgerEndpoint,
			Region:    config.LedgerRegion,
			Bucket:    config.LedgerBucket,
			AccessKey: config.LedgerAccessKey,
			SecretKey: config.LedgerSecretKey,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize ledger: %w", err)
		}
		history = objectStorage
	}

	slots, err := app.NewPlanner(catalogue, history).Plan(ctx, from, days)
	if err != nil {
		return err
	}
	if catalogue.Source() == site.SourceSnapshot {
		fmt.Fprintln(os.Stderr, "Note: the site was unreachable, this plan uses the built-in snapshot.")
	}
	if history == nil {
		fmt.Fprintln(os.Stderr, "Note: no ledger configured, posts featured before the plan starts are not known.")
	}
	return writePlan(os.Stdout, slots)
}

// parsePlanStart reads --from as YYYY-MM-DD or MM-DD in now's year.
func parsePlanStart(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return now, nil
	}
	if t, err := time.Parse(domain.DateLayout, value); err == nil {
		return t, nil
	}
	if t, err := time.Parse(domain.DateLayout, fmt.Sprintf("%d-%s", now.Year(), value)); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("--from must be YYYY-MM-DD or MM-DD (got %q)", value)
}

// writePlan prints the slots as a table, one day per row.
func writePlan(w io.Writer, slots []domain.Slot) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DATE\tDAY\tPROGRAM\tSCORE\tPOST\tWHY")
	rest := 0
	for _, s := range slots {
		post, score := "-", "-"
		if !s.IsRestDay() {
			post = s.Post.Article().Title
			if post == "" {
				post = s.Post.URI()
			}
			post = fmt.Sprintf("%s (%d)", post, s.Post.Date().Year())
			score = fmt.Sprintf("%.0f", s.Score)
		} else {
			rest++
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Date.Format(domain.DateLayout), s.Date.Format("Mon"), s.Program, score, post, strings.Join(s.Reasons, ", "))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\n%d days, %d posts, %d rest days\n", len(slots), len(slots)-rest, rest)
	return err
}

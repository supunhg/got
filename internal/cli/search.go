package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/store"
)

// newSearchCmd returns the `got search` command.
func newSearchCmd() *cobra.Command {
	var itemType string
	var limit int
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search decisions and notes",
		Long: `Search across architectural decision records and freeform notes.

Searches decision titles, context, decision body, alternatives, and
consequences, as well as note messages. Results are ranked by relevance
(number of matching fields) then recency.

Examples:
  got search SQLite
  got search Bubbletea --type decision
  got search WAL --type note
  got search database --limit 5
  got search CLI --json`,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, args[0], itemType, limit, jsonOut)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&itemType, "type", "", "Filter by type: decision, note")
	flags.IntVar(&limit, "limit", 0, "Max results (default 20, 0=default)")
	flags.BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func runSearch(cmd *cobra.Command, query, itemType string, limit int, jsonOut bool) error {
	ctx := context.Background()

	kc, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer kc.Close()
	ks := kc.ks

	// Validate type filter.
	var typePtr *string
	if itemType != "" {
		switch itemType {
		case "decision", "note":
			typePtr = &itemType
		default:
			return fmt.Errorf("search: invalid type %q: must be one of: decision, note", itemType)
		}
	}

	params := store.SearchParams{
		Query: query,
		Type:  typePtr,
		Limit: limit,
	}

	results, err := ks.Search(ctx, params)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if jsonOut {
		if results == nil {
			results = []store.SearchResult{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	return outputSearchResults(cmd, results)
}

func outputSearchResults(cmd *cobra.Command, results []store.SearchResult) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)

	if len(results) == 0 {
		fmt.Fprintln(w, "No results found.")
		return w.Flush()
	}

	fmt.Fprintln(w, "TYPE\tTITLE\tSTATUS\tWORKSPACE\tSCORE\tCREATED")

	for _, r := range results {
		ws := ""
		if r.WorkspaceID != nil {
			ws = *r.WorkspaceID
		}

		// Indicate type with badges.
		var typeLabel string
		switch r.Type {
		case "decision":
			typeLabel = "DEC"
		case "note":
			typeLabel = "NOTE"
		default:
			typeLabel = strings.ToUpper(r.Type)
		}

		date := time.UnixMilli(r.CreatedAt).Format("2006-01-02")
		title := truncate(r.Title, 50)
		status := r.Status
		if status == "" {
			status = "-"
		}
		score := fmt.Sprintf("%d", r.Score)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", typeLabel, title, status, ws, score, date)
	}

	return w.Flush()
}

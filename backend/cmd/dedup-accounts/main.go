// Command dedup-accounts scans anthropic accounts for duplicate upstream
// account_uuid records and reports/cleans them up.
//
// Default is dry-run: it only prints the plan (which row to keep, which to
// soft-delete). Pass --apply to actually soft-delete the duplicates.
//
// Keep policy: keep the earliest-created row of each uuid group (it usually
// carries the longest history / stickiest cache), soft-delete the rest.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"

	_ "github.com/lib/pq"
)

type accountRow struct {
	ID        int64
	UUID      string
	Name      string
	CreatedAt time.Time
}

type dedupGroup struct {
	UUID      string
	KeepID    int64
	KeepName  string
	RemoveIDs []int64
}

type dedupPlan struct {
	Groups []dedupGroup
}

// planDedup groups rows by uuid, keeps the earliest-created row, marks the rest
// for removal. Rows without a uuid are ignored.
func planDedup(rows []accountRow) dedupPlan {
	byUUID := map[string][]accountRow{}
	for _, r := range rows {
		if r.UUID == "" {
			continue
		}
		byUUID[r.UUID] = append(byUUID[r.UUID], r)
	}
	var plan dedupPlan
	for uuid, group := range byUUID {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			if group[i].CreatedAt.Equal(group[j].CreatedAt) {
				return group[i].ID < group[j].ID
			}
			return group[i].CreatedAt.Before(group[j].CreatedAt)
		})
		keep := group[0]
		remove := make([]int64, 0, len(group)-1)
		for _, r := range group[1:] {
			remove = append(remove, r.ID)
		}
		plan.Groups = append(plan.Groups, dedupGroup{
			UUID: uuid, KeepID: keep.ID, KeepName: keep.Name, RemoveIDs: remove,
		})
	}
	sort.Slice(plan.Groups, func(i, j int) bool { return plan.Groups[i].UUID < plan.Groups[j].UUID })
	return plan
}

func loadAnthropicRows(db *sql.DB) ([]accountRow, error) {
	rows, err := db.Query(`
		SELECT id, COALESCE(credentials->>'account_uuid', ''), name, created_at
		FROM accounts
		WHERE deleted_at IS NULL AND platform = 'anthropic'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []accountRow
	for rows.Next() {
		var r accountRow
		if err := rows.Scan(&r.ID, &r.UUID, &r.Name, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func main() {
	apply := flag.Bool("apply", false, "actually soft-delete duplicates (default: dry-run, print plan only)")
	flag.Parse()

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	db, err := sql.Open("postgres", cfg.Database.DSN())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := loadAnthropicRows(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load rows: %v\n", err)
		os.Exit(1)
	}
	plan := planDedup(rows)

	if len(plan.Groups) == 0 {
		fmt.Println("No duplicate anthropic account_uuid found. Nothing to do.")
		return
	}

	var totalRemove int
	fmt.Printf("Found %d duplicate uuid group(s):\n\n", len(plan.Groups))
	for _, g := range plan.Groups {
		fmt.Printf("uuid=%s  KEEP id=%d (%s)  REMOVE ids=%v\n", g.UUID, g.KeepID, g.KeepName, g.RemoveIDs)
		totalRemove += len(g.RemoveIDs)
	}
	fmt.Printf("\nTotal rows to remove: %d\n", totalRemove)

	if !*apply {
		fmt.Println("\n[dry-run] no changes made. Re-run with --apply to soft-delete the duplicates above.")
		return
	}

	var removed int
	for _, g := range plan.Groups {
		for _, id := range g.RemoveIDs {
			res, err := db.Exec(`UPDATE accounts SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "soft-delete id=%d failed: %v\n", id, err)
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				removed += int(n)
			}
		}
	}
	fmt.Printf("\n[apply] soft-deleted %d duplicate account row(s).\n", removed)
	fmt.Println("Next: apply migration 154 to add the unique index that prevents future dupes.")
}

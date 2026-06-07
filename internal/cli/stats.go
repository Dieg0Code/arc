package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/Dieg0Code/nem/internal/timing"
	"github.com/spf13/cobra"
)

// newStatsCmd crea `nem stats`: una tabla por proyecto con el tiempo activo real
// vs el span de calendario, sesiones y última actividad. Le da al agente (y al
// humano) conciencia temporal de en qué se trabajó y cuánto tardó de verdad.
func newStatsCmd() *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Per-project time stats: active work vs calendar, sessions, recency",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStats(cmd, days)
		},
	}
	cmd.Flags().IntVar(&days, "days", 0, "only projects active within the last N days (0 = all)")
	return cmd
}

func runStats(cmd *cobra.Command, days int) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	// Los nodos raíz del árbol SON los proyectos; ya traen las duraciones.
	projects, err := store.RootNodes()
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	cutoff := int64(0)
	if days > 0 {
		cutoff = now - int64(days)*86400
	}

	rows := projects[:0:0]
	for _, p := range projects {
		if p.Kind != "project" {
			continue
		}
		if cutoff > 0 && p.LastActive < cutoff {
			continue
		}
		rows = append(rows, p)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].LastActive > rows[j].LastActive })

	out := cmd.OutOrStdout()
	if len(rows) == 0 {
		fmt.Fprintln(out, "no indexed projects yet (run 'nem index')")
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tCHATS\tACTIVE\tCALENDAR\tSESSIONS\tLAST")
	var totActive time.Duration
	totSessions := 0
	for _, p := range rows {
		nchats := 0
		if ch, e := store.ChildNodes(p.ID); e == nil {
			for _, c := range ch {
				if c.Kind == "chat" {
					nchats++
				}
			}
		}
		active := time.Duration(p.ActiveSecs) * time.Second
		wall := time.Duration(p.WallSecs) * time.Second
		fmt.Fprintf(tw, "%s\t%d\t~%s\t%s\t%d\t%s\n",
			p.Title, nchats, timing.Format(active), timing.Format(wall), p.Sessions, timing.Ago(p.LastActive, now))
		totActive += active
		totSessions += p.Sessions
	}
	tw.Flush()
	fmt.Fprintf(out, "\ntotal: ~%s active across %d projects (%d sessions)\n",
		timing.Format(totActive), len(rows), totSessions)
	return nil
}

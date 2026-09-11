package headroom

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// WriteJSON emits the report as stable, indented JSON.
func WriteJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteTable emits a human-readable table plus a per-severity summary.
func WriteTable(w io.Writer, r Report) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tQUOTA\tRESOURCE\tUSED\tHARD\tPERCENT\tSEVERITY")
	for _, f := range r.Findings {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%.2f%%\t%s\n",
			f.Namespace, f.Quota, f.Resource, f.Used, f.Hard, f.Percent, f.Severity)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	keys := make([]string, 0, len(r.Summary))
	for k := range r.Summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, r.Summary[k]))
	}
	fmt.Fprintf(w, "\nSummary: %s\n", strings.Join(parts, "  "))
	return nil
}

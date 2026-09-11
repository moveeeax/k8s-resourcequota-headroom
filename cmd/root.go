package cmd

import (
	"github.com/spf13/cobra"
)

// version is set via -ldflags at build time.
var version = "dev"

// NewRootCmd builds the root command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "k8s-resourcequota-headroom",
		Short: "Audit Kubernetes ResourceQuota headroom before workloads get rejected",
		Long: "k8s-resourcequota-headroom reads every ResourceQuota and reports " +
			"used vs hard, percent consumed, and which namespaces are about to hit " +
			"their ceiling — while you can still do something about it.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	root.AddCommand(newScanCmd())
	return root
}

package cmd

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"

	"github.com/moveeeax/k8s-resourcequota-headroom/internal/headroom"
	"github.com/spf13/cobra"
)

type scanOpts struct {
	namespace     string
	allNamespaces bool
	warn          float64
	critical      float64
	output        string
	failOn        string
	input         string
	kubeconfig    string
}

func newScanCmd() *cobra.Command {
	o := &scanOpts{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan ResourceQuotas and report headroom",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd, o)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.namespace, "namespace", "n", "", "namespace to scan (default: current context namespace)")
	f.BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "scan all namespaces")
	f.Float64Var(&o.warn, "warn", 80, "percent-consumed at which a resource is LOW HEADROOM")
	f.Float64Var(&o.critical, "critical", 95, "percent-consumed at which a resource is CRITICAL")
	f.StringVarP(&o.output, "output", "o", "table", "output format: table|json")
	f.StringVar(&o.failOn, "fail-on", "", "exit non-zero when a finding at this level exists: warn|critical")
	f.StringVar(&o.input, "input", "", "read ResourceQuotas from a JSON/YAML dump instead of the cluster")
	f.StringVar(&o.kubeconfig, "kubeconfig", "", "path to kubeconfig (live mode)")
	return cmd
}

func runScan(cmd *cobra.Command, o *scanOpts) error {
	if o.warn <= 0 || o.critical <= 0 || o.warn > o.critical {
		return fmt.Errorf("invalid thresholds: need 0 < warn <= critical (got warn=%.1f critical=%.1f)", o.warn, o.critical)
	}
	gate, ok := headroom.ParseFailOn(o.failOn)
	if !ok {
		return fmt.Errorf("invalid --fail-on %q: use warn|critical|none", o.failOn)
	}

	var quotas []corev1.ResourceQuota
	var err error
	if o.input != "" {
		data, rerr := os.ReadFile(o.input)
		if rerr != nil {
			return fmt.Errorf("read --input: %w", rerr)
		}
		quotas, err = headroom.LoadDump(data)
	} else {
		clientset, cerr := headroom.NewClientset(o.kubeconfig)
		if cerr != nil {
			return fmt.Errorf("build client: %w", cerr)
		}
		ns := o.namespace
		if o.allNamespaces {
			ns = ""
		}
		quotas, err = headroom.FetchQuotas(context.Background(), clientset, ns)
	}
	if err != nil {
		return err
	}

	report := headroom.Classify(quotas, headroom.Thresholds{Warn: o.warn, Critical: o.critical})

	out := cmd.OutOrStdout()
	switch o.output {
	case "json":
		if err := headroom.WriteJSON(out, report); err != nil {
			return err
		}
	case "table":
		if err := headroom.WriteTable(out, report); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid --output %q: use table|json", o.output)
	}

	if report.ShouldFail(gate) {
		return &gateError{}
	}
	return nil
}

// gateError signals a CI gate failure and maps to exit code 2.
type gateError struct{}

func (*gateError) Error() string { return "headroom gate exceeded" }

// IsGateError reports whether err is a gate failure.
func IsGateError(err error) bool {
	_, ok := err.(*gateError)
	return ok
}

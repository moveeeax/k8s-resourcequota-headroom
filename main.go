// Command k8s-resourcequota-headroom audits Kubernetes ResourceQuota usage
// against hard limits and flags namespaces running out of headroom.
package main

import (
	"fmt"
	"os"

	"github.com/moveeeax/k8s-resourcequota-headroom/cmd"
)

func main() {
	root := cmd.NewRootCmd()
	if err := root.Execute(); err != nil {
		if cmd.IsGateError(err) {
			// Report already printed; gate failure is exit code 2.
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// Package headroom classifies Kubernetes ResourceQuota usage against its hard
// limits and flags namespaces that are running out of room before workloads
// start getting rejected at admission.
//
// The classifier is a pure function over ResourceQuota objects so it can be
// unit-tested entirely offline against fixtures, with no cluster access.
package headroom

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Severity ranks how close a resource is to its hard limit.
type Severity int

const (
	// SeverityOK means the resource is below the warn threshold.
	SeverityOK Severity = iota
	// SeverityWarn means the resource is at or above the warn threshold but
	// below critical: LOW HEADROOM.
	SeverityWarn
	// SeverityCritical means the resource is at or above the critical threshold.
	SeverityCritical
)

// String renders a severity as an upper-case label.
func (s Severity) String() string {
	switch s {
	case SeverityCritical:
		return "CRITICAL"
	case SeverityWarn:
		return "LOW HEADROOM"
	default:
		return "OK"
	}
}

// Thresholds are the warn/critical percentages (0-100) at which a resource is
// flagged. A resource is CRITICAL at or above Critical, WARN at or above Warn.
type Thresholds struct {
	Warn     float64
	Critical float64
}

// DefaultThresholds returns the built-in 80% warn / 95% critical gate.
func DefaultThresholds() Thresholds {
	return Thresholds{Warn: 80, Critical: 95}
}

// Finding is one resource line within one ResourceQuota.
type Finding struct {
	Namespace string  `json:"namespace"`
	Quota     string  `json:"quota"`
	Resource  string  `json:"resource"`
	Used      string  `json:"used"`
	Hard      string  `json:"hard"`
	Percent   float64 `json:"percent"`
	Severity  string  `json:"severity"`

	// usedVal/hardVal keep the parsed quantities for deterministic sorting and
	// are not part of the JSON contract.
	usedVal int64 `json:"-"`
	hardVal int64 `json:"-"`
}

// Report is the full classification result over a set of ResourceQuotas.
type Report struct {
	Findings []Finding      `json:"findings"`
	Summary  map[string]int `json:"summary"`
}

// percentOf returns 100*used/hard, rounded to two decimals. A zero hard limit
// yields 0 (an unlimited resource has infinite headroom).
func percentOf(used, hard int64) float64 {
	if hard <= 0 {
		return 0
	}
	p := float64(used) / float64(hard) * 100
	// round to 2 decimals
	return float64(int64(p*100+0.5)) / 100
}

// classify maps a percent to a severity given the thresholds.
func classify(percent float64, t Thresholds) Severity {
	switch {
	case percent >= t.Critical:
		return SeverityCritical
	case percent >= t.Warn:
		return SeverityWarn
	default:
		return SeverityOK
	}
}

// Classify computes a Finding for every tracked resource across the given
// ResourceQuotas and returns a stable, sorted Report. Resources present in
// status.hard drive the output; status.used is treated as zero when absent.
//
// CPU (milli) and memory (binary) quantities are compared correctly because
// every value is parsed through resource.Quantity.
func Classify(quotas []corev1.ResourceQuota, t Thresholds) Report {
	findings := make([]Finding, 0)
	summary := map[string]int{"OK": 0, "LOW HEADROOM": 0, "CRITICAL": 0}

	for _, q := range quotas {
		hard := q.Status.Hard
		used := q.Status.Used
		if len(hard) == 0 {
			// Fall back to spec.hard when status has not been populated yet
			// (e.g. an offline dump captured before the controller filled it).
			hard = q.Spec.Hard
		}
		for name, hardQty := range hard {
			usedQty := used[name]
			uv := quantityValue(usedQty)
			hv := quantityValue(hardQty)
			pct := percentOf(uv, hv)
			sev := classify(pct, t)
			findings = append(findings, Finding{
				Namespace: q.Namespace,
				Quota:     q.Name,
				Resource:  string(name),
				Used:      usedQty.String(),
				Hard:      hardQty.String(),
				Percent:   pct,
				Severity:  sev.String(),
				usedVal:   uv,
				hardVal:   hv,
			})
			summary[sev.String()]++
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		// Most-consumed first within a stable namespace/quota/resource order.
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Quota != b.Quota {
			return a.Quota < b.Quota
		}
		return a.Resource < b.Resource
	})

	return Report{Findings: findings, Summary: summary}
}

// quantityValue returns a comparable integer for a Quantity. Millivalue keeps
// sub-unit CPU quantities (e.g. 500m) meaningful while integer memory byte
// counts stay exact within int64 range.
func quantityValue(q resource.Quantity) int64 {
	// MilliValue multiplies by 1000; for large byte quantities this can
	// overflow, so fall back to Value for anything above a safe ceiling.
	const safe = int64(1) << 52
	if q.Value() > safe {
		return q.Value() * 1000
	}
	return q.MilliValue()
}

// FailOn is the gate level for CI.
type FailOn int

const (
	// FailNever never fails on findings.
	FailNever FailOn = iota
	// FailWarn fails when any warn or critical finding exists.
	FailWarn
	// FailCritical fails only when a critical finding exists.
	FailCritical
)

// ShouldFail reports whether the report violates the gate.
func (r Report) ShouldFail(gate FailOn) bool {
	switch gate {
	case FailWarn:
		return r.Summary["LOW HEADROOM"] > 0 || r.Summary["CRITICAL"] > 0
	case FailCritical:
		return r.Summary["CRITICAL"] > 0
	default:
		return false
	}
}

// ParseFailOn maps a flag string to a FailOn value; empty/"none" disables.
func ParseFailOn(s string) (FailOn, bool) {
	switch s {
	case "", "none":
		return FailNever, true
	case "warn":
		return FailWarn, true
	case "critical":
		return FailCritical, true
	default:
		return FailNever, false
	}
}

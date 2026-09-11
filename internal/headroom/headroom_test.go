package headroom

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// quota is a small helper to build a ResourceQuota with status.used/hard.
func quota(ns, name string, hard, used map[string]string) corev1.ResourceQuota {
	toList := func(m map[string]string) corev1.ResourceList {
		rl := corev1.ResourceList{}
		for k, v := range m {
			rl[corev1.ResourceName(k)] = resource.MustParse(v)
		}
		return rl
	}
	return corev1.ResourceQuota{
		TypeMeta:   metaType(),
		ObjectMeta: objMeta(ns, name),
		Status: corev1.ResourceQuotaStatus{
			Hard: toList(hard),
			Used: toList(used),
		},
	}
}

func TestClassify_Thresholds(t *testing.T) {
	q := quota("team-a", "quota", map[string]string{
		"cpu":    "10",   // 4 used -> 40% -> OK
		"memory": "10Gi", // 8.5Gi used -> 85% -> LOW HEADROOM
		"pods":   "20",   // 19 used -> 95% -> CRITICAL
	}, map[string]string{
		"cpu":    "4",
		"memory": "8704Mi", // 8.5Gi
		"pods":   "19",
	})
	r := Classify([]corev1.ResourceQuota{q}, DefaultThresholds())

	got := map[string]string{}
	pct := map[string]float64{}
	for _, f := range r.Findings {
		got[f.Resource] = f.Severity
		pct[f.Resource] = f.Percent
	}

	if got["cpu"] != "OK" {
		t.Errorf("cpu: want OK, got %s (%.2f%%)", got["cpu"], pct["cpu"])
	}
	if got["memory"] != "LOW HEADROOM" {
		t.Errorf("memory: want LOW HEADROOM, got %s (%.2f%%)", got["memory"], pct["memory"])
	}
	if got["pods"] != "CRITICAL" {
		t.Errorf("pods: want CRITICAL, got %s (%.2f%%)", got["pods"], pct["pods"])
	}
	if pct["cpu"] != 40 {
		t.Errorf("cpu percent: want 40, got %.2f", pct["cpu"])
	}
	if pct["pods"] != 95 {
		t.Errorf("pods percent: want 95, got %.2f", pct["pods"])
	}
}

func TestClassify_MilliCPU(t *testing.T) {
	// 750m used of 1 CPU hard = 75% (proves milli quantities compare right).
	q := quota("team-b", "q", map[string]string{"cpu": "1"}, map[string]string{"cpu": "750m"})
	r := Classify([]corev1.ResourceQuota{q}, DefaultThresholds())
	if len(r.Findings) != 1 {
		t.Fatalf("want 1 finding, got %d", len(r.Findings))
	}
	if r.Findings[0].Percent != 75 {
		t.Errorf("want 75%%, got %.2f%%", r.Findings[0].Percent)
	}
	if r.Findings[0].Severity != "OK" {
		t.Errorf("want OK, got %s", r.Findings[0].Severity)
	}
}

func TestClassify_BinaryMemory(t *testing.T) {
	// 1536Mi of 2Gi = 75%.
	q := quota("m", "q", map[string]string{"memory": "2Gi"}, map[string]string{"memory": "1536Mi"})
	r := Classify([]corev1.ResourceQuota{q}, DefaultThresholds())
	if r.Findings[0].Percent != 75 {
		t.Errorf("want 75%%, got %.2f%%", r.Findings[0].Percent)
	}
}

func TestShouldFail(t *testing.T) {
	crit := Report{Summary: map[string]int{"CRITICAL": 1, "LOW HEADROOM": 0, "OK": 3}}
	warnOnly := Report{Summary: map[string]int{"CRITICAL": 0, "LOW HEADROOM": 2, "OK": 1}}
	clean := Report{Summary: map[string]int{"CRITICAL": 0, "LOW HEADROOM": 0, "OK": 5}}

	if !crit.ShouldFail(FailWarn) || !crit.ShouldFail(FailCritical) {
		t.Error("critical report should fail both warn and critical gates")
	}
	if !warnOnly.ShouldFail(FailWarn) {
		t.Error("warn report should fail warn gate")
	}
	if warnOnly.ShouldFail(FailCritical) {
		t.Error("warn-only report must NOT fail critical gate")
	}
	if clean.ShouldFail(FailWarn) || clean.ShouldFail(FailCritical) {
		t.Error("clean report must never fail")
	}
	if clean.ShouldFail(FailNever) || crit.ShouldFail(FailNever) {
		t.Error("FailNever must never fail")
	}
}

func TestParseFailOn(t *testing.T) {
	cases := map[string]struct {
		want FailOn
		ok   bool
	}{
		"":         {FailNever, true},
		"none":     {FailNever, true},
		"warn":     {FailWarn, true},
		"critical": {FailCritical, true},
		"bogus":    {FailNever, false},
	}
	for in, exp := range cases {
		got, ok := ParseFailOn(in)
		if got != exp.want || ok != exp.ok {
			t.Errorf("ParseFailOn(%q) = (%v,%v), want (%v,%v)", in, got, ok, exp.want, exp.ok)
		}
	}
}

func TestClassify_ZeroHardIsUnlimited(t *testing.T) {
	q := quota("z", "q", map[string]string{"cpu": "0"}, map[string]string{"cpu": "5"})
	r := Classify([]corev1.ResourceQuota{q}, DefaultThresholds())
	if r.Findings[0].Percent != 0 || r.Findings[0].Severity != "OK" {
		t.Errorf("zero hard should be 0%% OK, got %.2f%% %s", r.Findings[0].Percent, r.Findings[0].Severity)
	}
}

func TestClassify_StableSort(t *testing.T) {
	qs := []corev1.ResourceQuota{
		quota("zeta", "q", map[string]string{"cpu": "10"}, map[string]string{"cpu": "1"}),
		quota("alpha", "q", map[string]string{"pods": "10", "cpu": "10"}, map[string]string{"pods": "1", "cpu": "1"}),
	}
	r := Classify(qs, DefaultThresholds())
	// alpha before zeta; within alpha, cpu before pods.
	order := []string{}
	for _, f := range r.Findings {
		order = append(order, f.Namespace+"/"+f.Resource)
	}
	want := []string{"alpha/cpu", "alpha/pods", "zeta/cpu"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("sort order = %v, want %v", order, want)
		}
	}
}

func TestJSONStable(t *testing.T) {
	q := quota("a", "q", map[string]string{"cpu": "4"}, map[string]string{"cpu": "3"})
	r := Classify([]corev1.ResourceQuota{q}, DefaultThresholds())
	var buf bytes.Buffer
	if err := WriteJSON(&buf, r); err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatalf("json round-trip: %v", err)
	}
	if len(back.Findings) != 1 || back.Findings[0].Percent != 75 {
		t.Errorf("round-trip lost data: %+v", back)
	}
}

func TestLoadDump_List(t *testing.T) {
	data := []byte(`{
	  "kind":"ResourceQuotaList","apiVersion":"v1",
	  "items":[{"kind":"ResourceQuota","metadata":{"name":"q","namespace":"team-a"},
	    "status":{"hard":{"cpu":"10"},"used":{"cpu":"10"}}}]}`)
	qs, err := LoadDump(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 || qs[0].Namespace != "team-a" {
		t.Fatalf("bad load: %+v", qs)
	}
	r := Classify(qs, DefaultThresholds())
	if r.Findings[0].Severity != "CRITICAL" {
		t.Errorf("10/10 cpu should be CRITICAL, got %s", r.Findings[0].Severity)
	}
}

func TestLoadDump_YAMLSingle(t *testing.T) {
	data := []byte("kind: ResourceQuota\napiVersion: v1\nmetadata:\n  name: q\n  namespace: ns\nstatus:\n  hard:\n    pods: \"10\"\n  used:\n    pods: \"8\"\n")
	qs, err := LoadDump(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 {
		t.Fatalf("want 1, got %d", len(qs))
	}
	r := Classify(qs, DefaultThresholds())
	if r.Findings[0].Percent != 80 || r.Findings[0].Severity != "LOW HEADROOM" {
		t.Errorf("8/10 pods = 80%% LOW HEADROOM, got %.2f%% %s", r.Findings[0].Percent, r.Findings[0].Severity)
	}
}

func TestWriteTable(t *testing.T) {
	q := quota("a", "q", map[string]string{"cpu": "10"}, map[string]string{"cpu": "10"})
	r := Classify([]corev1.ResourceQuota{q}, DefaultThresholds())
	var buf bytes.Buffer
	if err := WriteTable(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "CRITICAL") || !strings.Contains(out, "Summary:") {
		t.Errorf("table missing expected content:\n%s", out)
	}
}

package headroom

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// LoadDump parses ResourceQuota objects from an offline dump. It accepts:
//
//   - a corev1.ResourceQuotaList (kubectl get resourcequota -o yaml/json)
//   - a bare JSON/YAML array of ResourceQuota objects
//   - a single ResourceQuota object
//
// Both JSON and YAML input are supported (YAML is converted to JSON first).
func LoadDump(data []byte) ([]corev1.ResourceQuota, error) {
	j, err := yaml.YAMLToJSON(data)
	if err != nil {
		// Not YAML; try as raw JSON.
		j = data
	}

	// Try a List first.
	var list corev1.ResourceQuotaList
	if err := json.Unmarshal(j, &list); err == nil && list.Kind == "ResourceQuotaList" {
		return list.Items, nil
	}

	// Try a bare array.
	var arr []corev1.ResourceQuota
	if err := json.Unmarshal(j, &arr); err == nil && len(arr) > 0 {
		return arr, nil
	}

	// Try a single object.
	var single corev1.ResourceQuota
	if err := json.Unmarshal(j, &single); err == nil && single.Kind == "ResourceQuota" {
		return []corev1.ResourceQuota{single}, nil
	}

	// Last resort: a List even without an explicit Kind.
	if len(list.Items) > 0 {
		return list.Items, nil
	}

	return nil, fmt.Errorf("could not parse ResourceQuota dump: expected a ResourceQuotaList, an array, or a single ResourceQuota")
}

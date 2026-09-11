package headroom

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClientset builds a Kubernetes clientset from the standard kubeconfig
// resolution rules (KUBECONFIG env, --kubeconfig, then in-cluster).
func NewClientset(kubeconfig string) (kubernetes.Interface, error) {
	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loading.ExplicitPath = kubeconfig
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loading, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

// FetchQuotas lists ResourceQuotas from the cluster. An empty namespace lists
// across all namespaces.
func FetchQuotas(ctx context.Context, cs kubernetes.Interface, namespace string) ([]corev1.ResourceQuota, error) {
	list, err := cs.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

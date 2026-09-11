package headroom

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func metaType() metav1.TypeMeta {
	return metav1.TypeMeta{Kind: "ResourceQuota", APIVersion: "v1"}
}

func objMeta(ns, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: ns, Name: name}
}

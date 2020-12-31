package kube

import (
	"context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func NewScaler(clientset *kubernetes.Clientset) *Scaler {
	return &Scaler{clientset: clientset}
}

type Scaler struct {
	clientset *kubernetes.Clientset
}

func (s *Scaler) Scale(ctx context.Context, namespace, deployment string) (err error) {
	client := s.clientset.AppsV1().Deployments(namespace)
	scale, err := client.GetScale(ctx, deployment, v1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if scale.Spec.Replicas != 0 {
		return nil
	}
	scale.Spec.Replicas = 1
	if _, err = client.UpdateScale(ctx, deployment, scale, v1.UpdateOptions{}); apierrors.IsNotFound(err) {
		return nil
	}
	return
}

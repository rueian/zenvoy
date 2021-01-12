package kube

import (
	"context"

	"github.com/envoyproxy/go-control-plane/pkg/log"
	"golang.org/x/sync/singleflight"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ScaleMutation func(current int32) (expect int32)

var ScaleFromZero ScaleMutation = func(current int32) (expect int32) {
	if current == 0 {
		return 1
	}
	return current
}

var ScaleToZero ScaleMutation = func(current int32) (expect int32) {
	return 0
}

func NewScaler(logger log.Logger, clientset *kubernetes.Clientset, namespace string) *Scaler {
	return &Scaler{logger: logger, clientset: clientset, namespace: namespace}
}

type Scaler struct {
	logger    log.Logger
	clientset *kubernetes.Clientset
	namespace string

	sg singleflight.Group
}

func (s *Scaler) ScaleToZero(cluster string) {
	s.Scale(context.Background(), cluster, ScaleToZero)
}

func (s *Scaler) ScaleFromZero(cluster string) {
	s.Scale(context.Background(), cluster, ScaleFromZero)
}

func (s *Scaler) Scale(ctx context.Context, deployment string, mutate ScaleMutation) (err error) {
	_, err, _ = s.sg.Do(deployment, func() (interface{}, error) {
		client := s.clientset.AppsV1().Deployments(s.namespace)
		scale, err := client.GetScale(ctx, deployment, v1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		if expect := mutate(scale.Spec.Replicas); expect != scale.Spec.Replicas {
			scale.Spec.Replicas = expect
			if _, err = client.UpdateScale(ctx, deployment, scale, v1.UpdateOptions{}); apierrors.IsNotFound(err) {
				return nil, nil
			}
			s.logger.Infof("xds scaled deploy/%s to %d replicas", deployment, scale.Spec.Replicas)
		}
		return nil, err
	})
	return
}

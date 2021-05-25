package kube

import (
	"context"

	"github.com/envoyproxy/go-control-plane/pkg/log"
	"golang.org/x/sync/singleflight"
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

func NewScaler(logger log.Logger, clientset *kubernetes.Clientset, namespace string) Scaler {
	return &scaler{logger: logger, clientset: clientset, namespace: namespace}
}

type Scaler interface {
	ScaleToZero(cluster string)
	ScaleFromZero(cluster string)
}

type scaler struct {
	logger    log.Logger
	clientset *kubernetes.Clientset
	namespace string

	sg singleflight.Group
}

func (s *scaler) ScaleToZero(cluster string) {
	if err := s.scale(context.Background(), cluster, ScaleToZero); err != nil {
		s.logger.Errorf("failed to xds scaled deploy/%s to zero: %v", cluster, err)
	} else {
		s.logger.Infof("xds scaled deploy/%s to zero", cluster)
	}
}

func (s *scaler) ScaleFromZero(cluster string) {
	if err := s.scale(context.Background(), cluster, ScaleFromZero); err != nil {
		s.logger.Errorf("failed to xds scaled deploy/%s from zero: %v", cluster, err)
	} else {
		s.logger.Infof("xds scaled deploy/%s from zero", cluster)
	}
}

func (s *scaler) scale(ctx context.Context, deployment string, mutate ScaleMutation) (err error) {
	_, err, _ = s.sg.Do(deployment, func() (interface{}, error) {
		client := s.clientset.AppsV1().Deployments(s.namespace)
		scale, err := client.GetScale(ctx, deployment, v1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if expect := mutate(scale.Spec.Replicas); expect != scale.Spec.Replicas {
			scale.Spec.Replicas = expect
			_, err = client.UpdateScale(ctx, deployment, scale, v1.UpdateOptions{})
		}
		return nil, err
	})
	return
}

package controllers

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func NewControllerManager(namespace string) (manager.Manager, error) {
	conf, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	return manager.New(conf, manager.Options{Scheme: scheme, Namespace: namespace})
}

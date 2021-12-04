package xds

import (
	"context"
	"strings"

	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rueian/zenvoy/pkg/alloc"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func SetupEndpointController(mgr manager.Manager, monitor *MonitorServer, snapshot *Snapshot, proxyIP string, portMin, portMax uint32, vhFn EnvoyVirtualHostFn) error {
	controller := &EndpointController{
		Client:   mgr.GetClient(),
		monitor:  monitor,
		snapshot: snapshot,
		portsMap: alloc.NewKeys(portMin, portMax),
		proxyIP:  proxyIP,
		vhFn:     vhFn,
	}
	return builder.ControllerManagedBy(mgr).
		For(&v1.Endpoints{}).
		Complete(controller)
}

type EnvoyVirtualHostFn func(endpoints *v1.Endpoints) (*route.VirtualHost, error)

type EndpointController struct {
	client.Client
	monitor  *MonitorServer
	snapshot *Snapshot
	portsMap *alloc.Keys
	proxyIP  string
	vhFn     EnvoyVirtualHostFn
}

func (c *EndpointController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	endpoints := &v1.Endpoints{}
	if err := c.Get(ctx, req.NamespacedName, endpoints); err != nil {
		if apierrors.IsNotFound(err) {
			c.monitor.RemoveCluster(req.Name)
			c.portsMap.Release(req.Name)
			c.snapshot.RemoveClusterRoute(req.Name)
			c.snapshot.RemoveClusterEndpoints(req.Name)
			c.snapshot.RemoveCluster(req.Name)
			return reconcile.Result{}, nil
		} else {
			return reconcile.Result{}, err
		}
	}

	vh, err := c.vhFn(endpoints)
	if err != nil {
		return reconcile.Result{}, err
	}
	if vh == nil {
		return reconcile.Result{}, nil
	}

	var count int
	var available []Endpoint
	for _, sub := range endpoints.Subsets {
		port := c.findEndpointPort(endpoints, sub)
		for _, addr := range sub.Addresses {
			available = append(available, Endpoint{IP: addr.IP, Port: uint32(port)})
		}
		count += len(sub.Addresses) + len(sub.NotReadyAddresses)
	}

	c.monitor.TrackCluster(req.Name, count)

	if len(available) == 0 {
		port, err := c.portsMap.Acquire(req.Name)
		if err != nil {
			return reconcile.Result{Requeue: true}, nil
		}
		available = append(available, Endpoint{IP: c.proxyIP, Port: port})
	}

	c.snapshot.SetCluster(req.Name)
	c.snapshot.SetClusterRoute(req.Name, vh)
	c.snapshot.SetClusterEndpoints(req.Name, available...)
	return reconcile.Result{}, nil
}

func (c *EndpointController) findEndpointPort(endpoints *v1.Endpoints, subset v1.EndpointSubset) int32 {
	if len(subset.Ports) == 1 {
		return subset.Ports[0].Port
	}
	for _, port := range subset.Ports {
		switch strings.ToLower(port.Name) {
		case "http", "https", "http2", "tcp":
			return port.Port
		}
	}
	for _, port := range subset.Ports {
		switch port.Protocol {
		case "TCP":
			return port.Port
		}
	}
	return 0
}

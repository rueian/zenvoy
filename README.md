# zenvoy

![circleci](https://circleci.com/gh/rueian/zenvoy.svg?style=shield)
[![Maintainability](https://api.codeclimate.com/v1/badges/9706ade6af266b20323c/maintainability)](https://codeclimate.com/github/rueian/zenvoy/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/9706ade6af266b20323c/test_coverage)](https://codeclimate.com/github/rueian/zenvoy/test_coverage)

A L4 TCP TPROXY and a XDS server that supports envoy upstreams (including api servers or even databases) to be scaled from zero and also be scaled to zero.

## Demo
```
# deploy the demo

▶ kubectl apply -f example/minikube/kube.yaml

▶ kubectl get po
NAME                            READY   STATUS        RESTARTS   AGE
zenvoy-proxy-74cbb84c5-rhhl8    2/2     Running       0          2s   <- this is the L4 TCP ingress
zenvoy-xds-867674b7c8-v6thp     1/1     Running       0          2s   <- this is the XDS controlling the ingress

# note that wa have only the ingress and the XDS controller. we don't have the active upstream.

▶ kubectl port-forward svc/zenvoy-proxy 10000
Forwarding from 127.0.0.1:10000 -> 10000
Forwarding from [::1]:10000 -> 10000

# now, we send a request to the ingress with no active upstream, but get the response back.
# that is because the XDS scaled the upstream for us.

▶ curl -H 'HOST: zenvoy-echo' http://localhost:10000
GET / HTTP/1.1
Host: zenvoy-echo
Accept: */*
Content-Length: 0
User-Agent: curl/7.64.1

# re-check our pods, the upstream zenvoy-echo shows up.

▶ kubectl get po
NAME                           READY   STATUS    RESTARTS   AGE
zenvoy-echo-64b65fb6b6-vq6x9   1/1     Running   0          1s   <- this is the real upstream just be auto scaled up
zenvoy-proxy-74cbb84c5-rhhl8   2/2     Running   0          10s
zenvoy-xds-867674b7c8-v6thp    1/1     Running   0          10s

# wait 5 minutes later, the XDS scaled it down for us.

▶ kubectl get po
NAME                           READY   STATUS    RESTARTS   AGE
zenvoy-proxy-74cbb84c5-rhhl8   2/2     Running   0          5m
zenvoy-xds-867674b7c8-v6thp    1/1     Running   0          5m
```

## How it works

The `zenvoy-proxy` is an Envoy + TCP TProxy. They are both connected to the XDS controller
which is watching kubernetes `Endpoints` resources.

When XDS controller received an `Endpoints` resource with no healthy pods, it allocates a port mapping
for the corresponding `Endpoints` and publish the envoy config to the `zenvoy-proxy`.

When the request hits the Envoy of `zenvoy-proxy`, it will be redirected to the TProxy and waits for the real upstream
to be scaled up by the XDS controller after receiving the metrics from the Envoy.

Once the real upstream is scaled up, the TProxy uses the port mapping to redirect pending request to the real upstream.
And the TProxy is also removed from the Envoy config at the same time by the XDS controller.
Future requests will go to real upstream directly.

After a configurable timeout, the XDS controller will scale down the upstream if there is no more requests sent to the upstream.
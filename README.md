# zenvoy
A L4 tcp proxy and a XDS server that supports envoy upstream endpoints to scale from&to zero.   

```
▶ kubectl apply -f example/minikube/kube.yaml

▶ kubectl get po
NAME                            READY   STATUS        RESTARTS   AGE
zenvoy-proxy-74cbb84c5-rhhl8    2/2     Running       0          2s
zenvoy-xds-867674b7c8-v6thp     1/1     Running       0          2s

▶ kubectl port-forward svc/zenvoy-proxy 10000
Forwarding from 127.0.0.1:10000 -> 10000
Forwarding from [::1]:10000 -> 10000

▶ curl -H 'HOST: zenvoy-echo' http://localhost:10000
GET / HTTP/1.1
Host: zenvoy-echo
Accept: */*
Content-Length: 0
User-Agent: curl/7.64.1

▶ kubectl get po
NAME                           READY   STATUS    RESTARTS   AGE
zenvoy-echo-64b65fb6b6-vq6x9   1/1     Running   0          1s
zenvoy-proxy-74cbb84c5-rhhl8   2/2     Running   0          10s
zenvoy-xds-867674b7c8-v6thp    1/1     Running   0          10s

# 5 minutes later

▶ kubectl get po
NAME                           READY   STATUS    RESTARTS   AGE
zenvoy-proxy-74cbb84c5-rhhl8   2/2     Running   0          5m
zenvoy-xds-867674b7c8-v6thp    1/1     Running   0          5m
```

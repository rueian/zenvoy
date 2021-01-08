package config

import (
	"github.com/kelseyhightower/envconfig"
	"time"
)

type XDS struct {
	XDSPort          uint32        `envconfig:"XDS_PORT"       default:"18000"`
	XDSNodeID        string        `envconfig:"XDS_NODE_ID"    default:"zenvoy"`
	ProxyAddr        string        `envconfig:"PROXY_ADDR"     default:"127.0.0.1"`
	ProxyPortMin     uint32        `envconfig:"PROXY_PORT_MIN" default:"20000"`
	ProxyPortMax     uint32        `envconfig:"PROXY_PORT_MAX" default:"32767"`
	KubeNamespace    string        `envconfig:"KUBE_NAMESPACE" default:"default"`
	ScaleToZeroAfter time.Duration `envconfig:"SCALE_TO_ZERO_AFTER" default:"5m"`
	ScaleToZeroCheck time.Duration `envconfig:"SCALE_TO_ZERO_CHECK" default:"30s"`
}

type Proxy struct {
	ProxyAddr    string `envconfig:"PROXY_ADDR"     default:"127.0.0.1"`
	ProxyPort    uint32 `envconfig:"PROXY_PORT"     default:"18001"`
	ProxyPortMin uint32 `envconfig:"PROXY_PORT_MIN" default:"20000"`
	ProxyPortMax uint32 `envconfig:"PROXY_PORT_MAX" default:"32767"`
	XDSAddr      string `envconfig:"XDS_ADDR"       default:"zenvoy-xds:18000"`
	XDSNodeID    string `envconfig:"XDS_NODE_ID"    default:"zenvoy"`
	TPROXYCMD    string `envconfig:"TPROXY_CMD"     default:"iptables"`
}

func GetXDS() (out XDS, err error) {
	err = envconfig.Process("", &out)
	return
}

func GetProxy() (out Proxy, err error) {
	err = envconfig.Process("", &out)
	return
}

package config

import "github.com/kelseyhightower/envconfig"

type XDS struct {
	XDSPort       uint32 `envconfig:"XDS_PORT"       default:"18000"`
	XDSNodeID     string `envconfig:"XDS_NODE_ID"    default:"zenvoy"`
	TriggerPort   uint32 `envconfig:"TRIGGER_PORT"   default:"17999"`
	ProxyAddr     string `envconfig:"PROXY_ADDR"     default:"127.0.0.1"`
	ProxyPortMin  uint32 `envconfig:"PROXY_PORT_MIN" default:"20000"`
	ProxyPortMax  uint32 `envconfig:"PROXY_PORT_MAX" default:"32767"`
	KubeNamespace string `envconfig:"KUBE_NAMESPACE" default:"default"`
}

type Proxy struct {
	ProxyAddr    string `envconfig:"PROXY_ADDR"     default:"127.0.0.1"`
	ProxyPort    uint32 `envconfig:"PROXY_PORT"     default:"18001"`
	ProxyPortMin uint32 `envconfig:"PROXY_PORT_MIN" default:"20000"`
	ProxyPortMax uint32 `envconfig:"PROXY_PORT_MAX" default:"32767"`
	XDSAddr      string `envconfig:"XDS_ADDR"       default:"xds:18000"`
	XDSNodeID    string `envconfig:"XDS_NODE_ID"    default:"zenvoy"`
	TriggerURL   string `envconfig:"TRIGGER_URL"    default:"http://xds:17999"`
}

func GetXDS() (out XDS, err error) {
	err = envconfig.Process("", &out)
	return
}

func GetProxy() (out Proxy, err error) {
	err = envconfig.Process("", &out)
	return
}

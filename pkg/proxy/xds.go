package proxy

type XDS interface {
	GetIntendedEndpoints(port uint32) []string
	OnUpdated(func())
}

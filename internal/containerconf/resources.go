package containerconf

type ResourceType int

const (
	ResourceTypeUnknown ResourceType = iota
	ResourceTypeFile
	ResourceTypeFeature
	ResourceTypeLocal
)

const (
	KindPubKey     = "pub_key"
	KindSharedKey  = "shared_key"
	KindAuthFile   = "auth"
	KindDockerfile = "dockerfile"
)

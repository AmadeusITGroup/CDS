package bo

// NOTE: Should be only used in host and db packages as biz object interface between these two packages.
// ANY OTHER USAGE SHOULD BE PROHIBITED

type Host struct {
	Name     string
	Username string
	Password string
	KeyPair
	Port      int
	IsDefault bool
}
type KeyPair struct {
	PathToPub string
	PathToPrv string
}

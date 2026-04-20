package output

// SimpleResult is a single-message result for commands with no complex output.
type SimpleResult struct {
	Message string `json:"message"`
}

func (r SimpleResult) HumanReadable(_ OutputOptions) string {
	return r.Message + "\n"
}

func (r SimpleResult) MachineReadable() any { return r }

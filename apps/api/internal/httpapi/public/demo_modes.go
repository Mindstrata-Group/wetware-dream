package public

// DemoModeOption is the public projection of an AI mode shown before login.
type DemoModeOption struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	DemoChat string `json:"demoChat"`
}

func CloneDemoModes(src []DemoModeOption) []DemoModeOption {
	dst := make([]DemoModeOption, len(src))
	copy(dst, src)
	return dst
}

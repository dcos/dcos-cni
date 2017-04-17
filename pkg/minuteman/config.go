package minuteman

type NetConf struct {
	Minuteman *config `json:"minuteman, omitempty"`
}

type config struct {
	Path string `json:"path, omitempty"`
}

package version

var (
	Version   = "0.1.0-dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

func Current() Info {
	return Info{Version: Version, Commit: Commit, BuildTime: BuildTime}
}


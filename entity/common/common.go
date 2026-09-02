package common

// VersionResponse carries application version info.
type VersionResponse struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// SettingItem represents a single app setting entry.
type SettingItem struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// HealthResponse carries the liveness probe result.
type HealthResponse struct {
	Status     string `json:"status"`
	ServerTime string `json:"server_time"`
}

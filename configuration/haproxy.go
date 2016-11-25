package configuration

type HAProxy struct {
	TemplatePath            string
	OutputPath              string
	OutputDir               string
	ReloadCommand           string
	ReloadValidationCommand string
	ReloadCleanupCommand    string
	IP                      string
	Port                    string
	UiPort                  string
	BackendMaxConn          string
}

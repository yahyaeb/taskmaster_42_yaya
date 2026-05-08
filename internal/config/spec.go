package config

type ConfigSpec struct {
	ProcessName   string            `yaml:"process_name"`
	Program       string            `yaml:"program"`
	Cmd           string            `yaml:"cmd"`
	Numprocs      int               `yaml:"numprocs"`
	NumprocsStart int               `yaml:"numprocs_start"`
	Umask         int               `yaml:"umask"`
	Workingdir    string            `yaml:"workingdir"`
	Autostart     bool              `yaml:"autostart"`
	Autorestart   string            `yaml:"autorestart"`
	Exitcodes     []int             `yaml:"exitcodes"`
	Startretries  int               `yaml:"startretries"`
	Starttime     int               `yaml:"starttime"`
	Stopsignal    string            `yaml:"stopsignal"`
	Stoptime      int               `yaml:"stoptime"`
	Stdout        string            `yaml:"stdout"`
	Stderr        string            `yaml:"stderr"`
	Env           map[string]string `yaml:"env"`
}

type Loader interface {
	Load(path string) ([]ConfigSpec, error)
}

package cenv

import (
	"os"
	"path/filepath"
	"runtime"

	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	// KConfigPathEnvVar is the name of the environment variable that can be used to override the default configuration path
	kConfigPathEnvVar = "CDS_CONFIG_PATH"

	kClientConfigDirName = ".xcds"
	kAgentConfigDirName  = ".xcds-agent"
)

var (
	sDataDir = kClientConfigDirName
)

func SetConfigDirForAgent() {
	sDataDir = kAgentConfigDirName
}

func ConfigFile(filename string) string {
	return configPath(filename)
}

func ConfigDir(dirname string) string {
	return configPath(dirname)
}

func GlobalConfigPath() string {
	return ConfigDir(cg.EmptyStr)
}

func configPath(filename string) string {
	if dir := os.Getenv(kConfigPathEnvVar); dir != "" {
		return filepath.Join(dir, sDataDir, filename)
	}
	homedir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(homedir, sDataDir, filename)
}

// determines the users based on local ENV variables
// TODO:Feature: handle edge cases, eg when root in some containers, $USER is undefined
func GetUsernameFromEnv() string {
	var user string
	switch runtime.GOOS {
	case "windows":
		user = os.Getenv("USERNAME")
	default:
		user = os.Getenv("USER")
	}

	if len(user) == 0 {
		return "cdsanonymous"
	} else {
		return user
	}
}

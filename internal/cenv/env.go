package cenv

import (
	"os"
	"path/filepath"
	"runtime"

	cg "github.com/amadeusitgroup/cds/internal/global"
)

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
	if dir := os.Getenv("CDS_CONFIG_PATH"); dir != "" {
		return filepath.Join(dir, ".xcds", filename)
	}
	homedir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(homedir, ".xcds", filename)
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

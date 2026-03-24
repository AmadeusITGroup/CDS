package containerconf

import (
	"io"
	"regexp"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

var (
	baseConf *ConfigApp
)

type ConfigApp struct {
	v *viper.Viper
}

func instance() *ConfigApp {
	if baseConf == nil {
		baseConf = new(ConfigApp)
		baseConf.v = viper.New()
		return baseConf
	}
	return baseConf
}

// LoadFromBytes loads a devcontainer.json configuration from raw bytes into
// the global Viper instance. This is used by the agent to receive configuration
// from the CLI client instead of reading it from a local file.
func LoadFromBytes(dataReader io.Reader) error {
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return cerr.AppendError("Couldn't unload configuration reader", err)
	}
	// drop any comment line, inline comments are not allowed
	re := regexp.MustCompile(`(?m:^\s*//.*$)`)
	uncommented := re.ReplaceAll(data, nil)

	instance().v.SetConfigType("json")
	if err := instance().v.ReadConfig(strings.NewReader(string(uncommented))); err != nil {
		return cerr.AppendError("Failed to parse configuration from bytes", err)
	}
	return nil
}

func WriteConfigToFile(path string) error {
	return instance().v.SafeWriteConfigAs(path)
}

func Get(key ...string) interface{} {
	return instance().v.Get(cg.VariadicJoin(".", key...))
}

func UnmarshalKey(key string, rawVal interface{}) {
	err := instance().v.UnmarshalKey(key, rawVal)
	if err != nil {
		clog.Debug("[containerconf.Unmarshal] Got a non-nil error", clog.NewLoggable("key", key), err)
	}
}

func IsSet(key ...string) bool {
	return instance().v.IsSet(cg.VariadicJoin(".", key...))
}

func Set(key string, value interface{}) {
	instance().v.Set(key, value)
}

func BindFlagToConfig(key string, flag *pflag.Flag) error {
	return instance().v.BindPFlag(key, flag)
}

func IsNasRequested() bool {
	mountNas, ok := Get(KCds, KCdsMountNas).(bool)

	return mountNas && ok
}

func IsRegistryRequested() bool {
	return IsSet(KOrchestration, KOrchestrationRegistry)
}

func GetOrchestrationConfigFilePath() string {
	if configFile, hasConfigFile := Get(KOrchestration, KOrchestrationConfigFile).(string); hasConfigFile {
		return configFile
	}
	return ""
}

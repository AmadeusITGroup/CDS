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

// Config holds one parsed devcontainer configuration. It is intentionally
// request-scoped so agent workflows can operate on explicit inputs instead of
// mutating global process state.
type Config struct {
	v *viper.Viper
}

// NewConfig returns an empty configuration instance.
func NewConfig() *Config {
	return &Config{v: viper.New()}
}

// ParseBytes loads a devcontainer.json configuration from raw bytes into a new
// configuration instance.
func ParseBytes(dataReader io.Reader) (*Config, error) {
	config := NewConfig()
	if err := config.LoadFromBytes(dataReader); err != nil {
		return nil, err
	}
	return config, nil
}

func (c *Config) ensureViper() *viper.Viper {
	if c.v == nil {
		c.v = viper.New()
	}
	return c.v
}

// LoadFromBytes loads a devcontainer.json configuration from raw bytes into the
// current config instance, replacing any previous values.
func (c *Config) LoadFromBytes(dataReader io.Reader) error {
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return cerr.AppendError("Couldn't unload configuration reader", err)
	}
	// Drop any full-line comment. Inline comments are not supported by the spec.
	re := regexp.MustCompile(`(?m:^\s*//.*$)`)
	uncommented := re.ReplaceAll(data, nil)
	if strings.TrimSpace(string(uncommented)) == "" {
		return cerr.NewError("configuration from bytes is empty")
	}

	next := viper.New()
	next.SetConfigType("json")
	if err := next.ReadConfig(strings.NewReader(string(uncommented))); err != nil {
		return cerr.AppendError("Failed to parse configuration from bytes", err)
	}
	c.v = next
	return nil
}

func (c *Config) WriteConfigToFile(path string) error {
	return c.ensureViper().SafeWriteConfigAs(path)
}

func (c *Config) Get(key ...string) interface{} {
	return c.ensureViper().Get(cg.VariadicJoin(".", key...))
}

func (c *Config) UnmarshalKey(key string, rawVal interface{}) {
	err := c.ensureViper().UnmarshalKey(key, rawVal)
	if err != nil {
		clog.Debug("[containerconf.Unmarshal] Got a non-nil error", clog.NewLoggable("key", key), err)
	}
}

func (c *Config) IsSet(key ...string) bool {
	return c.ensureViper().IsSet(cg.VariadicJoin(".", key...))
}

func (c *Config) Set(key string, value interface{}) {
	c.ensureViper().Set(key, value)
}

func (c *Config) BindFlagToConfig(key string, flag *pflag.Flag) error {
	return c.ensureViper().BindPFlag(key, flag)
}

func (c *Config) IsNasRequested() bool {
	mountNas, ok := c.Get(KCds, KCdsMountNas).(bool)
	return mountNas && ok
}

func (c *Config) IsRegistryRequested() bool {
	return c.IsSet(KOrchestration, KOrchestrationRegistry)
}

func (c *Config) GetOrchestrationConfigFilePath() string {
	if configFile, hasConfigFile := c.Get(KOrchestration, KOrchestrationConfigFile).(string); hasConfigFile {
		return configFile
	}
	return ""
}

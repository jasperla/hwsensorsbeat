// Config is put into a different package to prevent cyclic imports in case
// it is needed in several locations

package config

type Config struct {
	Hwsensorsbeat HwsensorsbeatConfig
}

type HwsensorsbeatConfig struct {
	Period  string   `yaml:"period"`
	Devices []string `yaml:"devices"`
}

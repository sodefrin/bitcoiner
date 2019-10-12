package config

import "github.com/kelseyhightower/envconfig"

var Version string

func GetEnv(env interface{}) error {
	if err := envconfig.Process("", env); err != nil {
		envconfig.Usage("", env)
		return err
	}
	return nil
}

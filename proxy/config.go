package proxy

import (
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Namespace       string `default:"cattle-system"`
	TLSName         string `split_words:"true"`
	CAName          string `required:"true" split_words:"true"`
	CertCANamespace string `required:"true" split_words:"true"`
	CertCAName      string `required:"true" split_words:"true"`
	Secret          string `required:"true" split_words:"true"`
	ProxyPort       int    `required:"true" split_words:"true"`
	PeerPort        int    `required:"true" split_words:"true"`
	HTTPSPort       int    `required:"true" split_words:"true"`
}

func ConfigFromEnvironment() (*Config, error) {
	var c Config
	err := envconfig.Process("", &c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

package remotedialer

import (
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"k8s.io/client-go/rest"
)

func BuildSecretController(restConfig *rest.Config) (v1.SecretController, error) {
	core, err := core.NewFactoryFromConfigWithOptions(restConfig, nil)
	if err != nil {
		return nil, err
	}

	return core.Core().V1().Secret(), nil
}

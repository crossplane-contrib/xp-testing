package images

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"github.com/maximilianbraun/xp-testing/pkg/envvar"
)

const (
	imagesJSONEnv = "E2E_IMAGES"
)

// ProviderImages holds information to the docker images for providers
type ProviderImages struct {
	Package         string
	ControllerImage *string
}

// GetImagesFromJSONOrPanic retrieves image information from the environment and panics if `E2E_IMAGES` is not set
// `E2E_IMAGES` is expected to be a simple json like this.
// ```{"$PackageKey": "ImageUrlOfPackageImage", "$controllerKey": "ImageUrlOfControllerImage"}```
// The controller image (key) is optional
func GetImagesFromJSONOrPanic(packageKey string, controllerKey *string) ProviderImages {
	imagesJSON := envvar.GetOrPanic(imagesJSONEnv)
	images := map[string]string{}

	err := json.Unmarshal([]byte(imagesJSON), &images)

	if err != nil {
		panic(errors.Wrap(err, fmt.Sprintf("failed to unmarshal json from %s", imagesJSONEnv)))
	}

	uutConfig := images[packageKey]

	var uutController *string
	if controllerKey != nil {
		val := images[*controllerKey]
		uutController = &val
	}

	return ProviderImages{
		Package:         uutConfig,
		ControllerImage: uutController,
	}
}

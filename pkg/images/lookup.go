package images

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/maximilianbraun/xp-testing/pkg/envvar"
)

const (
	imagesJSONEnv = "TEST_IMAGES"
)

// ProviderImages holds information to the docker images for providers
type ProviderImages struct {
	Package         string
	ControllerImage *string
}

// GetImagesFromJSONOrPanic retrieves image information from the environment and panics if `TEST_IMAGES` is not set
// `TEST_IMAGES` is expected to be a simple json like this.
// ```{"$PackageKey": "ImageUrlOfPackageImage", "$controllerKey": "ImageUrlOfControllerImage"}```
// The controller image (key) is optional
func GetImagesFromJSONOrPanic(packageKey string, controllerKey *string) ProviderImages {
	imagesJSON := envvar.GetOrPanic(imagesJSONEnv)
	images := map[string]string{}

	err := json.Unmarshal([]byte(imagesJSON), &images)

	if err != nil {
		panic(errors.Wrap(err, "failed to unmarshal json from UUT_IMAGE"))
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

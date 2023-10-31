package images

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LookupSuite struct {
	suite.Suite
}

func TestLookupTestSuite(t *testing.T) {
	suite.Run(t, new(LookupSuite))
}

func (suite *LookupSuite) TestGetImagesFromJSONOrPanic() {
	packageKey := "foo"
	controllerKey := "baz"

	suite.T().Run("Returns both images from environment", func(t *testing.T) {
		err := os.Setenv("E2E_IMAGES", "{\"foo\": \"bar\", \"baz\": \"boom\"}")
		println(err)
		providerImages := GetImagesFromEnvironmentOrPanic(packageKey, &controllerKey)
		require.Equal(t, "bar", providerImages.Package)
		require.Equal(t, "boom", *providerImages.ControllerImage)
	})

	suite.T().Run("Returns existing env vars", func(t *testing.T) {
		os.Setenv("E2E_IMAGES", "{\"foo\": \"bar\"}")
		providerImages := GetImagesFromEnvironmentOrPanic(packageKey, nil)
		require.Equal(t, "bar", providerImages.Package)
		require.Nil(t, providerImages.ControllerImage)
	})

	suite.T().Run("env var not set, will panic", func(t *testing.T) {
		os.Unsetenv("E2E_IMAGES")
		require.Panics(t, func() {
			GetImagesFromEnvironmentOrPanic(packageKey, nil)
		})
	})

	suite.T().Run("invalid json, will panic", func(t *testing.T) {
		os.Setenv("E2E_IMAGES", "//invalid.json")
		require.Panics(t, func() {
			GetImagesFromEnvironmentOrPanic(packageKey, nil)
		})
	})
	os.Unsetenv("E2E_IMAGES")
}

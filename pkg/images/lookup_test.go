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

	suite.Run("Returns both images from environment", func() {
		err := os.Setenv("E2E_IMAGES", "{\"foo\": \"bar\", \"baz\": \"boom\"}")
		println(err)
		providerImages := GetImagesFromEnvironmentOrPanic(packageKey, &controllerKey)
		require.Equal(suite.T(), "bar", providerImages.Package)
		require.Equal(suite.T(), "boom", *providerImages.ControllerImage)
	})

	suite.Run("Returns existing env vars", func() {
		os.Setenv("E2E_IMAGES", "{\"foo\": \"bar\"}")
		providerImages := GetImagesFromEnvironmentOrPanic(packageKey, nil)
		require.Equal(suite.T(), "bar", providerImages.Package)
		require.Nil(suite.T(), providerImages.ControllerImage)
	})

	suite.Run("env var not set, will panic", func() {
		os.Unsetenv("E2E_IMAGES")
		require.Panics(suite.T(), func() {
			GetImagesFromEnvironmentOrPanic(packageKey, nil)
		})
	})

	suite.Run("invalid json, will panic", func() {
		os.Setenv("E2E_IMAGES", "//invalid.json")
		require.Panics(suite.T(), func() {
			GetImagesFromEnvironmentOrPanic(packageKey, nil)
		})
	})
	os.Unsetenv("E2E_IMAGES")
}

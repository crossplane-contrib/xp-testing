package images

import (
	"os"
	"testing"

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
		suite.Require().Equal("bar", providerImages.Package)
		suite.Require().Equal("boom", *providerImages.ControllerImage)
	})

	suite.Run("Returns existing env vars", func() {
		os.Setenv("E2E_IMAGES", "{\"foo\": \"bar\"}")
		providerImages := GetImagesFromEnvironmentOrPanic(packageKey, nil)
		suite.Require().Equal("bar", providerImages.Package)
		suite.Require().Nil(providerImages.ControllerImage)
	})

	suite.Run("env var not set, will panic", func() {
		os.Unsetenv("E2E_IMAGES")
		suite.Require().Panics(func() {
			GetImagesFromEnvironmentOrPanic(packageKey, nil)
		})
	})

	suite.Run("invalid json, will panic", func() {
		os.Setenv("E2E_IMAGES", "//invalid.json")
		suite.Require().Panics(func() {
			GetImagesFromEnvironmentOrPanic(packageKey, nil)
		})
	})
	os.Unsetenv("E2E_IMAGES")
}

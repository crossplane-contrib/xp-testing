package images

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
		providerImages := GetImagesFromJSONOrPanic(packageKey, &controllerKey)
		assert.Equal(t, "bar", providerImages.Package)
		assert.Equal(t, "boom", *providerImages.ControllerImage)
	})

	suite.T().Run("Returns existing env vars", func(t *testing.T) {
		os.Setenv("E2E_IMAGES", "{\"foo\": \"bar\"}")
		providerImages := GetImagesFromJSONOrPanic(packageKey, nil)
		assert.Equal(t, "bar", providerImages.Package)
		assert.Nil(t, providerImages.ControllerImage)
	})

	suite.T().Run("env var not set, will panic", func(t *testing.T) {
		os.Unsetenv("E2E_IMAGES")
		assert.Panics(t, func() {
			GetImagesFromJSONOrPanic(packageKey, nil)
		})
	})

	suite.T().Run("invalid json, will panic", func(t *testing.T) {
		os.Setenv("E2E_IMAGES", "//invalid.json")
		assert.Panics(t, func() {
			GetImagesFromJSONOrPanic(packageKey, nil)
		})
	})
	os.Unsetenv("E2E_IMAGES")
}

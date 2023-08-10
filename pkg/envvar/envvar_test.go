package envvar

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type EnvVarTestSuite struct {
	suite.Suite
}

func (suite *EnvVarTestSuite) SetupTest() {
	os.Setenv("ENVVARTEST_EMPTY", "")
	os.Setenv("ENVVARTEST_SINGLE", "This is a single line")
	os.Setenv("ENVVARTEST_MULTILINE", `This
is
a multiline string!`)
}

func (suite *EnvVarTestSuite) TearDownTest() {
	for _, envvar := range os.Environ() {
		if strings.HasPrefix(envvar, "ENVVARTEST_") {
			os.Unsetenv(envvar)
		}
	}
}

func (suite *EnvVarTestSuite) TestGet() {
	suite.T().Run("Returns existing env vars", func(t *testing.T) {
		assert.Equal(t, "", Get("ENVVARTEST_EMPTY"))
		assert.Equal(t, "This is a single line", Get("ENVVARTEST_SINGLE"))
		assert.Equal(t, "This\nis\na multiline string!", Get("ENVVARTEST_MULTILINE"))
	})
	suite.T().Run("Returns empty string if env var can't be found", func(t *testing.T) {
		assert.Empty(suite.T(), Get("ENVVARTEST_DOESNT_EXIST"))
	})
}

func (suite *EnvVarTestSuite) TestGetOrDefault() {
	suite.T().Run("Returns existing env vars", func(t *testing.T) {
		assert.Equal(t, "", GetOrDefault("ENVVARTEST_EMPTY", ""))
		assert.Equal(t, "This is a single line", GetOrDefault("ENVVARTEST_SINGLE", ""))
		assert.Equal(t, "This\nis\na multiline string!", GetOrDefault("ENVVARTEST_MULTILINE", ""))
	})
	suite.T().Run("Returns default value if env var can't be found", func(t *testing.T) {
		assert.Equal(t, "a default value", GetOrDefault("ENVVARTEST_DOESNT_EXIST", "a default value"))
		assert.Equal(t, "another default value", GetOrDefault("ENVVARTEST_DOESNT_EXIST", "another default value"))
	})
}

func (suite *EnvVarTestSuite) TestGetOrPanic() {
	suite.T().Run("Returns existing env vars", func(t *testing.T) {
		assert.Equal(t, "", GetOrPanic("ENVVARTEST_EMPTY"))
		assert.Equal(t, "This is a single line", GetOrPanic("ENVVARTEST_SINGLE"))
		assert.Equal(t, "This\nis\na multiline string!", GetOrPanic("ENVVARTEST_MULTILINE"))
	})
	suite.T().Run("Panics if env var can't be found", func(t *testing.T) {
		assert.Panics(t, func() { GetOrPanic("ENVVARTEST_DOESNT_EXIST") })
		assert.Panics(t, func() { GetOrPanic("ENVVARTEST_DOESNT_EXIST") })
	})
}

func TestEnvVarTestSuite(t *testing.T) {
	suite.Run(t, new(EnvVarTestSuite))
}

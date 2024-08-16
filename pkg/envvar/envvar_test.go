package envvar

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	suite.Run("Returns existing env vars", func() {
		require.Equal(suite.T(), "", Get("ENVVARTEST_EMPTY"))
		require.Equal(suite.T(), "This is a single line", Get("ENVVARTEST_SINGLE"))
		require.Equal(suite.T(), "This\nis\na multiline string!", Get("ENVVARTEST_MULTILINE"))
	})
	suite.Run("Returns empty string if env var can't be found", func() {
		suite.Require().Empty(Get("ENVVARTEST_DOESNT_EXIST"))
	})
}

func (suite *EnvVarTestSuite) TestGetOrDefault() {
	suite.Run("Returns existing env vars", func() {
		require.Equal(suite.T(), "", GetOrDefault("ENVVARTEST_EMPTY", ""))
		require.Equal(suite.T(), "This is a single line", GetOrDefault("ENVVARTEST_SINGLE", ""))
		require.Equal(suite.T(), "This\nis\na multiline string!", GetOrDefault("ENVVARTEST_MULTILINE", ""))
	})
	suite.Run("Returns default value if env var can't be found", func() {
		require.Equal(suite.T(), "a default value", GetOrDefault("ENVVARTEST_DOESNT_EXIST", "a default value"))
		require.Equal(suite.T(), "another default value", GetOrDefault("ENVVARTEST_DOESNT_EXIST", "another default value"))
	})
}

func (suite *EnvVarTestSuite) TestGetOrPanic() {
	suite.Run("Returns existing env vars", func() {
		require.Equal(suite.T(), "", GetOrPanic("ENVVARTEST_EMPTY"))
		require.Equal(suite.T(), "This is a single line", GetOrPanic("ENVVARTEST_SINGLE"))
		require.Equal(suite.T(), "This\nis\na multiline string!", GetOrPanic("ENVVARTEST_MULTILINE"))
	})
	suite.Run("Panics if env var can't be found", func() {
		require.Panics(suite.T(), func() { GetOrPanic("ENVVARTEST_DOESNT_EXIST") })
		require.Panics(suite.T(), func() { GetOrPanic("ENVVARTEST_DOESNT_EXIST") })
	})
}

func TestEnvVarTestSuite(t *testing.T) {
	suite.Run(t, new(EnvVarTestSuite))
}

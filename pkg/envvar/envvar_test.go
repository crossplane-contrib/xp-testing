package envvar

import (
	"os"
	"strings"
	"testing"

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
		suite.Require().Empty(Get("ENVVARTEST_EMPTY"))
		suite.Require().Equal("This is a single line", Get("ENVVARTEST_SINGLE"))
		suite.Require().Equal("This\nis\na multiline string!", Get("ENVVARTEST_MULTILINE"))
	})
	suite.Run("Returns empty string if env var can't be found", func() {
		suite.Require().Empty(Get("ENVVARTEST_DOESNT_EXIST"))
	})
}

func (suite *EnvVarTestSuite) TestGetOrDefault() {
	suite.Run("Returns existing env vars", func() {
		suite.Require().Empty(GetOrDefault("ENVVARTEST_EMPTY", ""))
		suite.Require().Equal("This is a single line", GetOrDefault("ENVVARTEST_SINGLE", ""))
		suite.Require().Equal("This\nis\na multiline string!", GetOrDefault("ENVVARTEST_MULTILINE", ""))
	})
	suite.Run("Returns default value if env var can't be found", func() {
		suite.Require().Equal("a default value", GetOrDefault("ENVVARTEST_DOESNT_EXIST", "a default value"))
		suite.Require().Equal("another default value", GetOrDefault("ENVVARTEST_DOESNT_EXIST", "another default value"))
	})
}

func (suite *EnvVarTestSuite) TestGetOrPanic() {
	suite.Run("Returns existing env vars", func() {
		suite.Require().Empty(GetOrPanic("ENVVARTEST_EMPTY"))
		suite.Require().Equal("This is a single line", GetOrPanic("ENVVARTEST_SINGLE"))
		suite.Require().Equal("This\nis\na multiline string!", GetOrPanic("ENVVARTEST_MULTILINE"))
	})
	suite.Run("Panics if env var can't be found", func() {
		suite.Require().Panics(func() { GetOrPanic("ENVVARTEST_DOESNT_EXIST") })
		suite.Require().Panics(func() { GetOrPanic("ENVVARTEST_DOESNT_EXIST") })
	})
}

func TestEnvVarTestSuite(t *testing.T) {
	suite.Run(t, new(EnvVarTestSuite))
}

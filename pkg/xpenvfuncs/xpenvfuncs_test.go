package xpenvfuncs

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func TestCompose(t *testing.T) {
	incEnvFunc := func(i *int) env.Func {
		return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			*i++
			return ctx, nil
		}
	}

	errEnvFunc := func(err error) env.Func {
		return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			return ctx, err
		}
	}

	t.Run(
		"nop", func(t *testing.T) {
			pctx := context.Background()

			ctx, err := Compose()(pctx, nil)

			require.NoError(t, err)
			require.Equal(t, pctx, ctx)
		},
	)

	t.Run(
		"passes ctx and cfg to child envfunc", func(t *testing.T) {
			invoked := false

			pctx := context.Background()
			pcfg := &envconf.Config{}

			ctx, err := Compose(
				func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
					invoked = true
					require.Equal(t, pctx, ctx)
					require.Equal(t, pcfg, cfg)
					return ctx, nil
				},
			)(pctx, pcfg)

			require.True(t, invoked)
			require.NoError(t, err)
			require.Equal(t, pctx, ctx)
		},
	)

	t.Run(
		"invokes all configured child funcs", func(t *testing.T) {
			invocations := 0

			_, err := Compose(
				incEnvFunc(&invocations),
				incEnvFunc(&invocations),
				incEnvFunc(&invocations),
			)(context.Background(), &envconf.Config{})

			require.NoError(t, err)
			require.Equal(t, 3, invocations)
		},
	)

	t.Run(
		"stops processing in case of error", func(t *testing.T) {
			invocations := 0

			_, err := Compose(
				incEnvFunc(&invocations),
				incEnvFunc(&invocations),
				errEnvFunc(fmt.Errorf("stop here")),
				incEnvFunc(&invocations),
			)(context.Background(), &envconf.Config{})

			require.EqualError(t, err, "stop here")
			require.Equal(t, 2, invocations)
		},
	)
}

func TestGetClusterControlPlaneName(t *testing.T) {
	require.Equal(t, "my-cluster-control-plane", getClusterControlPlaneName("my-cluster"))
}

func TestRenderTemplate(t *testing.T) {
	type args struct {
		template string
		data     interface{}
	}
	type expects struct {
		rendered     string
		errorMessage string
	}

	tests := []struct {
		description string
		args        args
		expects     expects
	}{
		{
			description: "nop",
		},
		{
			description: "simple string without replacements",
			args: args{
				template: "This is a simple string and nothing should be replaced!",
			},
			expects: expects{
				rendered: "This is a simple string and nothing should be replaced!",
			},
		},
		{
			description: "simple string with single replacement",
			args: args{
				template: "Hello {{.Subject}}!",
				data: struct {
					Subject string
				}{
					Subject: "World",
				},
			},
			expects: expects{
				rendered: "Hello World!",
			},
		},
		{
			description: "multiline string with multiple replacements",
			args: args{
				template: `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: {{.Name}}
spec:
  package: {{.Package}}
  packagePullPolicy: Never`,
				data: struct {
					Name    string
					Package string
				}{
					Name:    "my-provider",
					Package: "my-registry.local/path/to/my-provider:1.2.3",
				},
			},
			expects: expects{
				rendered: `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: my-provider
spec:
  package: my-registry.local/path/to/my-provider:1.2.3
  packagePullPolicy: Never`,
			},
		},
		{
			description: "multiline string with condition is true",
			args: args{
				template: `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: {{.Name}}
spec:
  package: {{.Package}}
  packagePullPolicy: Never
  {{- if .ControllerConfig }}
  controllerConfigRef:
    name: {{.ControllerConfig}}
{{end}}`,
				data: struct {
					Name             string
					Package          string
					ControllerConfig string
				}{
					Name:             "my-provider",
					Package:          "my-registry.local/path/to/my-provider:1.2.3",
					ControllerConfig: "my-controller-config-ref",
				},
			},
			expects: expects{
				rendered: `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: my-provider
spec:
  package: my-registry.local/path/to/my-provider:1.2.3
  packagePullPolicy: Never
  controllerConfigRef:
    name: my-controller-config-ref
`,
			},
		},
		{
			description: "multiline string with condition is false",
			args: args{
				template: `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: {{.Name}}
spec:
  package: {{.Package}}
  packagePullPolicy: Never
  {{- if .ControllerConfig }}
  controllerConfigRef:
    name: {{.ControllerConfig}}
{{end}}`,
				data: struct {
					Name             string
					Package          string
					ControllerConfig string
				}{
					Name:    "my-provider",
					Package: "my-registry.local/path/to/my-provider:1.2.3",
				},
			},
			expects: expects{
				rendered: `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: my-provider
spec:
  package: my-registry.local/path/to/my-provider:1.2.3
  packagePullPolicy: Never`,
			},
		},
		{
			description: "with runtimeConfigRef",
			args: args{
				template: `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: {{.Name}}
spec:
  package: {{.Package}}
  packagePullPolicy: Never
  {{- if .ControllerConfig }}
  runtimeConfigRef:
    name: {{.RuntimeConfig}}
  {{end}}`,
				data: struct {
					Name          string
					Package       string
					RuntimeConfig string
				}{
					Name:          "my-provider",
					Package:       "my-registry.local/path/to/my-provider:1.2.3",
					RuntimeConfig: "my-runtime-config-ref",
				},
			},
			expects: expects{
				rendered: `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: my-provider
spec:
  package: my-registry.local/path/to/my-provider:1.2.3
  packagePullPolicy: Never
  runtimeConfigRef:
    name: my-runtime-config-ref
`,
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.description, func(t *testing.T) {
				rendered, err := renderTemplate(test.args.template, test.args.data)

				if len(test.expects.errorMessage) == 0 {
					if err == nil {
						require.Equal(t, test.expects.rendered, rendered)
					}
				} else {
					require.Error(t, err)
				}
			},
		)
	}
}

var dummyErrFunc = func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	return ctx, errors.New("dummy err")
}

func TestDummyErr(t *testing.T) {
	t.Run(
		"dummyErrFunc returns error", func(t *testing.T) {
			_, err := dummyErrFunc(nil, nil)
			require.Error(t, err)
		},
	)
}

func TestConditional(t *testing.T) {
	t.Run(
		"executes", func(t *testing.T) {
			conditionalFn := Conditional(dummyErrFunc, true)
			_, err := conditionalFn(nil, nil)
			require.Error(t, err)
		},
	)
	t.Run(
		"doesnt execute", func(t *testing.T) {
			conditionalFn := Conditional(dummyErrFunc, false)
			_, err := conditionalFn(nil, nil)
			require.NoError(t, err)
		},
	)
}

func TestIgnoreErr(t *testing.T) {
	t.Run(
		"no error thown", func(t *testing.T) {
			conditionalFn := IgnoreErr(dummyErrFunc)
			_, err := conditionalFn(nil, nil)
			require.NoError(t, err)
		},
	)
}

func TestIgnoreMatchedErr(t *testing.T) {
	t.Run(
		"no error thown", func(t *testing.T) {
			conditionalFn := IgnoreMatchedErr(dummyErrFunc, func(err error) bool {
				return false
			})
			_, err := conditionalFn(nil, nil)
			require.NoError(t, err)
		},
	)

	t.Run(
		"error thown", func(t *testing.T) {
			conditionalFn := IgnoreMatchedErr(dummyErrFunc, func(err error) bool {
				return true
			})
			_, err := conditionalFn(nil, nil)
			require.NoError(t, err)
		},
	)
}

func Test_ValidateTestSetup(t *testing.T) {
	tests := []struct {
		name      string
		setup     ValidateTestSetupOptions
		wantError bool
	}{
		{
			name: "v2.0.0 setup with registry",
			setup: ValidateTestSetupOptions{
				"v2.0.0", "xpkg.crossplane.io", nil,
			},
			wantError: true,
		},
		{
			name: ">v2 setup with registry",
			setup: ValidateTestSetupOptions{
				"v2.1.0", "xpkg.crossplane.io", nil,
			},
			wantError: true,
		},
		{
			name: "<v2 setup with registry",
			setup: ValidateTestSetupOptions{
				"v1.20.1", "xpkg.crossplane.io", nil,
			},
			wantError: false,
		},
		{
			name: "v2 with controllerconfig",
			setup: ValidateTestSetupOptions{
				"v2.0.0", "", &vendored.ControllerConfig{},
			},
			wantError: true,
		},
		{
			name:      "implicit v2 without controllerconfig",
			setup:     ValidateTestSetupOptions{},
			wantError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envFunc := ValidateTestSetup(tt.setup)
			_, err := envFunc(nil, nil)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

package docker

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSave(t *testing.T) {
	type args struct {
		image       string
		target      string
		returnError error
	}
	type expects struct {
		options      []string
		errorMessage string
	}

	tests := []struct {
		description string
		args        args
		expects     expects
	}{
		{
			description: "happy path",
			args: args{
				image:  "alpine",
				target: "alpine.tar",
			},
			expects: expects{
				options: []string{"alpine", "-o", "alpine.tar"},
			},
		},
		{
			description: "returns an error if no args are given",
			expects: expects{
				errorMessage: "please provide source and target",
			},
		},
		{
			description: "returns an error if image is empty",
			args: args{
				target: "output.tar",
			},
			expects: expects{
				errorMessage: "please provide source and target",
			},
		},
		{
			description: "returns an error if target is empty",
			args: args{
				image: "alpine",
			},
			expects: expects{
				errorMessage: "please provide source and target",
			},
		},
		{
			description: "returns an error if command can't be executed",
			args: args{
				image:       "ubuntu",
				target:      "out.tar",
				returnError: fmt.Errorf("sth went wrong"),
			},
			expects: expects{
				options:      []string{"ubuntu", "-o", "out.tar"},
				errorMessage: "sth went wrong",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			runDocker = func(command string, options ...string) error {
				assert.Equal(t, "save", command)
				assert.Equal(t, test.expects.options, options)
				return test.args.returnError
			}

			err := Save(test.args.image, test.args.target)

			if len(test.expects.errorMessage) == 0 {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expects.errorMessage)
			}
		})
	}
}

func TestCp(t *testing.T) {
	type args struct {
		source      string
		target      string
		returnError error
	}
	type expects struct {
		options      []string
		errorMessage string
	}

	tests := []struct {
		description string
		args        args
		expects     expects
	}{
		{
			description: "happy path - copy from local to container",
			args: args{
				source: "file.tar",
				target: "dff106214482:/tmp/file.tar",
			},
			expects: expects{
				options: []string{"file.tar", "dff106214482:/tmp/file.tar"},
			},
		},
		{
			description: "happy path - copy from container to local",
			args: args{
				source: "dff106214482:/tmp/file.tar",
				target: "file.tar",
			},
			expects: expects{
				options: []string{"dff106214482:/tmp/file.tar", "file.tar"},
			},
		},
		{
			description: "returns an error if no args are given",
			expects: expects{
				errorMessage: "please provide source and target",
			},
		},
		{
			description: "returns an error if image is empty",
			args: args{
				target: "dff106214482:/home/ci/123.gz",
			},
			expects: expects{
				errorMessage: "please provide source and target",
			},
		},
		{
			description: "returns an error if target is empty",
			args: args{
				source: "some-file.txt",
			},
			expects: expects{
				errorMessage: "please provide source and target",
			},
		},
		{
			description: "returns an error if command can't be executed",
			args: args{
				source:      "another-file.log",
				target:      "container-name:/var/log/another-file.log",
				returnError: fmt.Errorf("sth went wrong"),
			},
			expects: expects{
				options:      []string{"another-file.log", "container-name:/var/log/another-file.log"},
				errorMessage: "sth went wrong",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			runDocker = func(command string, options ...string) error {
				assert.Equal(t, "cp", command)
				assert.Equal(t, test.expects.options, options)
				return test.args.returnError
			}

			err := Cp(test.args.source, test.args.target)

			if len(test.expects.errorMessage) == 0 {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expects.errorMessage)
			}
		})
	}
}

func TestExec(t *testing.T) {
	type args struct {
		container   string
		command     string
		options     []string
		returnError error
	}
	type expects struct {
		options      []string
		errorMessage string
	}

	tests := []struct {
		description string
		args        args
		expects     expects
	}{
		{
			description: "happy path - no options",
			args: args{
				container: "some-container",
				command:   "whoami",
			},
			expects: expects{
				options: []string{"some-container", "whoami"},
			},
		},
		{
			description: "happy path - complex command",
			args: args{
				container: "some-container",
				command:   "/bin/bash",
				options:   []string{"-c", "'find . -iname \"*.yaml\" -exec cat {} \\;'"},
			},
			expects: expects{
				options: []string{"some-container", "/bin/bash", "-c", "'find . -iname \"*.yaml\" -exec cat {} \\;'"},
			},
		},
		{
			description: "returns an error if no args are given",
			expects: expects{
				errorMessage: "please provide container and command",
			},
		},
		{
			description: "returns an error if container is not specified",
			args: args{
				command: "ping",
			},
			expects: expects{
				errorMessage: "please provide container and command",
			},
		},
		{
			description: "returns an error if command is not specified",
			args: args{
				container: "some-container",
			},
			expects: expects{
				errorMessage: "please provide container and command",
			},
		},

		{
			description: "returns an error if command can't be executed",
			args: args{
				container:   "some-container",
				command:     "whoami",
				returnError: fmt.Errorf("sth went wrong"),
			},
			expects: expects{
				options:      []string{"some-container", "whoami"},
				errorMessage: "sth went wrong",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			runDocker = func(command string, options ...string) error {
				assert.Equal(t, "exec", command)
				assert.Equal(t, test.expects.options, options)
				return test.args.returnError
			}

			err := Exec(test.args.container, test.args.command, test.args.options...)

			if len(test.expects.errorMessage) == 0 {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expects.errorMessage)
			}
		})
	}
}

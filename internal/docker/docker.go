package docker

import (
	"fmt"
	"strings"

	"github.com/vladimirvivien/gexe"
)

// Save runs docker save
func Save(image string, target string) error {
	if len(image) == 0 || len(target) == 0 {
		return fmt.Errorf("please provide source and target")
	}

	return runDocker("save", image, "-o", target)
}

// Cp runs docker cp
func Cp(src string, dest string) error {
	if len(src) == 0 || len(dest) == 0 {
		return fmt.Errorf("please provide source and target")
	}

	return runDocker("cp", src, dest)
}

// Exec runs docker exec
func Exec(container string, command string, options ...string) error {
	if len(container) == 0 || len(command) == 0 {
		return fmt.Errorf("please provide container and command")
	}

	return runDocker("exec", append([]string{container, command}, options...)...)
}

var runDocker = func(command string, options ...string) error {
	proc := gexe.RunProc(fmt.Sprintf("docker %s %s", command, strings.Join(options, " ")))

	if proc.ExitCode() != 0 {
		return fmt.Errorf("failed to execute 'docker %s': %s", command, proc.Result())
	}

	return nil
}

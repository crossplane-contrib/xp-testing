package xpkg

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"

	container "github.com/GoogleContainerTools/container-diff/pkg/util"
)

var extractContainerImage = func(image string, path string) error {
	_, err := container.GetImage(fmt.Sprintf("daemon://%s", image), false, path)
	return err
}

// FetchPackageContent returns the content of the package.yaml file from the givecn crossplane package
func FetchPackageContent(crossplanePackage string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "xpkg-*")
	if err != nil {
		return "", err
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tmpDir)

	if err = extractContainerImage(crossplanePackage, tmpDir); err != nil {
		return "", err
	}
	pkg, err := os.ReadFile(fmt.Sprintf("%s/package.yaml", tmpDir))
	if err != nil {
		return "", err
	}
	return string(pkg), nil

}

// SavePackage saves the crossplane package descriptor of the given image gzipped to the specified target file
func SavePackage(crossplanePackage string, targetFile string) error {
	pkg, err := FetchPackageContent(crossplanePackage)
	if err != nil {
		return err
	}
	// nolint: gosec
	f, err := os.OpenFile(targetFile, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	gzipWriter := gzip.NewWriter(f)
	writer := bufio.NewWriter(gzipWriter)
	_, _ = writer.WriteString(pkg)
	_ = writer.Flush()
	_ = gzipWriter.Close()
	return f.Close()
}

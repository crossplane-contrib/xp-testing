package xpkg

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/pkg/errors"
)

const (
	errFmtNoPackageFileFound = "couldn't find package.yaml file after checking %d files in the archive"
)

const packageFile = "package.yaml"

var (

	// required for testing with fake data
	extractContainerImage = extractPackageYamlFromImage
)

// tarFile represents a single file inside a tar. Closing it closes the tar itself.
type tarFile struct {
	io.Reader
	io.Closer
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
	pkg, err := os.ReadFile(fmt.Sprintf("%s/%s", tmpDir, packageFile))
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

// extractPackageYamlFromImage extracts the 'package.yaml' from the crossplane xpkg image.
// nolint:gocyclo
func extractPackageYamlFromImage(imageName, tempDirPath string) error {
	// this func has dependencies only on Google container registry and docker moby

	reference, err := name.ParseReference(imageName)
	if err != nil {
		return fmt.Errorf("error parsing image name %w", err)
	}
	// Save the Docker image to a tar file
	image, err := daemon.Image(reference)
	if err != nil {
		return err
	}

	tarc := mutate.Extract(image)

	localFile, errCreate := os.Create(filepath.Join(tempDirPath, filepath.Base(packageFile))) //nolint:gosec, we dictate path, not user provided
	if errCreate != nil {
		return fmt.Errorf("unable to create temp package.yaml file: %w", errCreate)
	}
	defer func(localFile *os.File) {
		_ = localFile.Close()
	}(localFile)

	// search for the desired file within the tarball, only on the current layer
	t := tar.NewReader(tarc)
	var read int
	for {
		h, err := t.Next()
		if err != nil {
			return errors.Wrapf(err, errFmtNoPackageFileFound, read)
		}
		if h.Name == packageFile {
			_, errCopy := io.Copy(localFile, t) //nolint:gosec
			if errCopy != nil {
				return fmt.Errorf("failed to copy content to local %s file: %w", packageFile, errCopy)
			}
			// if successfully errCopy is nil
			return nil
		}
		read++
	}
}

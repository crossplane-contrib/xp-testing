package xpkg

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"
)

const packageFile = "package.yaml"
const layerTar = "layer.tar"

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

	// Pull the Docker image
	if err := exec.Command("docker", "pull", imageName).Run(); err != nil {
		return fmt.Errorf("error pulling image: %w", err)
	}

	// Save the Docker image to a tar file
	var imageBuff bytes.Buffer
	saveCmd := exec.Command("docker", "save", imageName)
	saveCmd.Stdout = &imageBuff
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("error saving image: %w", err)
	}

	// require imageREader for multiple passes over io.ReadCloser/io.Reader
	// ps: no need to close as it is a wrapper around []byte
	imageReader := bytes.NewReader(imageBuff.Bytes())

	// retrieve manifest from the xpgk, we need this to get the layers digest and path
	layerPath, errPackageLayer := findPackageYamlInImage(ioOpener(io.NopCloser(imageReader)))
	if errPackageLayer != nil {
		return errPackageLayer
	}

	// reset to start
	_, err := imageReader.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("error seeking to start of imageReader: %w", err)
	}

	// e.g. layerPath := "6a19324dac365085b6cf6d286dc0afd4cba84f98ef896f512ecf58d5b9e1566c/layer.tar"
	layerRC, errF := extractFileFromTar(ioOpener(io.NopCloser(imageReader)), layerPath)
	if errF != nil {
		return errF
	}

	localFile, errCreate := os.Create(filepath.Join(tempDirPath, filepath.Base(packageFile))) //nolint:gosec, we dictate path, not user provided
	if errCreate != nil {
		return fmt.Errorf("unable to create temp package.yaml file: %w", errCreate)
	}
	defer func(localFile *os.File) {
		_ = localFile.Close()
	}(localFile)

	// search for the desired file within the tarball, only on the current layer
	tr := tar.NewReader(layerRC)
	for {
		header, errNext := tr.Next()
		if errNext == io.EOF {
			break
		}
		if errNext != nil {
			return fmt.Errorf("failed to read tar header from layer: %w", errNext)
		}
		if header.Name == packageFile {
			_, errCopy := io.Copy(localFile, tr) //nolint:gosec
			if errCopy != nil {
				return fmt.Errorf("failed to copy content to local %s file: %w", packageFile, errCopy)
			}
			// if successfully errCopy is nil
			return nil
		}
	}

	return errors.New(fmt.Sprintf("no %s file found in xpkg image", packageFile))
}

// extractFileFromTar
func extractFileFromTar(opener tarball.Opener, filePath string) (io.ReadCloser, error) {
	f, err := opener()
	if err != nil {
		return nil, err
	}
	needClose := true
	defer func() {
		if needClose {
			errClose := f.Close()
			if errClose != nil {
				return
			}
		}
	}()

	tf := tar.NewReader(f)
	for {
		hdr, err := tf.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == filePath {
			if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
				currentDir := filepath.Dir(filePath)
				return extractFileFromTar(opener, path.Join(currentDir, path.Clean(hdr.Linkname)))
			}
			needClose = false
			return tarFile{
				Reader: tf,
				Closer: f,
			}, nil
		}
	}
	return nil, fmt.Errorf("file %s not found in tar", filePath)
}

// findPackageYamlInImage finds the layer containing the package.yaml file.
func findPackageYamlInImage(opener tarball.Opener) (string, error) {
	f, err := opener()
	if err != nil {
		return "", err
	}
	needClose := true
	defer func() {
		if needClose {
			errClose := f.Close()
			if errClose != nil {
				return
			}
		}
	}()

	tarReader := tar.NewReader(f)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// Check if this is a layer
		if header.Typeflag == tar.TypeReg && strings.Contains(header.Name, layerTar) {
			if layerContainsPackageYaml(tarReader) {
				return header.Name, nil
			}
		}
	}
	return "", fmt.Errorf("%s not found in any layer", packageFile)
}

// layerContainsPackageYaml checks if the given layer contains the package.yaml file.
func layerContainsPackageYaml(layerReader io.Reader) bool {

	tarLayerReader := tar.NewReader(layerReader)
	for {
		header, err := tarLayerReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}

		if header.Typeflag == tar.TypeReg && header.Name == packageFile {
			return true
		}
	}
	return false
}

// ioOpener is a func for opening a tar file
func ioOpener(closer io.ReadCloser) tarball.Opener {
	return func() (io.ReadCloser, error) {
		return closer, nil
	}
}

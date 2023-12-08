package xpkg

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"
)

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

// extractPackageYamlFromImage extracts the 'package.yaml' from the crossplane xpkg image.
// nolint:gocyclo
func extractPackageYamlFromImage(imageName, tempDirPath string) error {
	// this func has dependencies only on Google container registry and docker moby

	ctx := context.Background()
	cli, errClient := client.NewClientWithOpts(client.FromEnv)
	if errClient != nil {
		return fmt.Errorf("error creating client to docker: %w", errClient)
	}

	imageReader, errSave := cli.ImageSave(ctx, []string{imageName})
	if errSave != nil {
		return fmt.Errorf("error saving xpkg image with docker: %w", errSave)
	}
	defer func(imageReader io.ReadCloser) {
		_ = imageReader.Close()
	}(imageReader)

	// require TeeReader for multiple passes over io.ReadCloser
	// once to fetch manifest, then again for fetching package.yaml from layer
	var buf bytes.Buffer
	tee := io.TeeReader(imageReader, &buf)

	// retrieve manifest from the xpgk, we need this to get the layers digest and path
	manifest, errM := tarball.LoadManifest(ioOpener(io.NopCloser(tee)))
	if errM != nil {
		return errM
	}

	// e.g. path := "6a19324dac365085b6cf6d286dc0afd4cba84f98ef896f512ecf58d5b9e1566c/layer.tar"
	layerPath := manifest[0].Layers[0]
	layerRC, errF := extractFileFromTar(ioOpener(io.NopCloser(&buf)), layerPath)
	if errF != nil {
		return errF
	}

	const packageFile = "package.yaml"
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
				return fmt.Errorf("failed to copy content to local package.yaml file: %w", errCopy)
			}
			// if successfully errCopy is nil
			return nil
		}
	}

	return errors.New("no package.yaml file found in xpkg image")
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

// ioOpener is a func for opening a tar file
func ioOpener(closer io.ReadCloser) tarball.Opener {
	return func() (io.ReadCloser, error) {
		return closer, nil
	}
}

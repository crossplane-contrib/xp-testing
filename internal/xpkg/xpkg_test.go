package xpkg

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const csdProvider = `apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: my-provider
spec:
  package: my-provider
  packagePullPolicy: Never`

func returnStaticXPKG(t *testing.T, expectedImage string, returnContent string, returnError error) func(string, string) error {
	return func(image string, path string) error {
		require.Equal(t, expectedImage, image)

		if returnError != nil {
			return returnError
		}

		f, err := os.OpenFile(fmt.Sprintf("%s/package.yaml", path), os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}

		writer := bufio.NewWriter(f)
		writer.WriteString(returnContent)
		writer.Flush()
		return f.Close()
	}
}

func TestFetchPackageContent(t *testing.T) {
	type args struct {
		crossplanePackage string
		returnContent     string
		returnError       error
	}
	type expects struct {
		crossplanePackageContent string
		errorMessage             string
	}

	tests := []struct {
		description string
		args        args
		expects     expects
	}{
		{
			description: "happy path",
			args: args{
				crossplanePackage: "build-7b9b1d70/crossplane/provider-abc",
				returnContent:     csdProvider,
			},
			expects: expects{
				crossplanePackageContent: csdProvider,
			},
		},
		{
			description: "error case",
			args: args{
				returnError: fmt.Errorf("sth went wrong"),
			},
			expects: expects{
				errorMessage: "sth went wrong",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			extractContainerImage = returnStaticXPKG(t, test.args.crossplanePackage, test.args.returnContent, test.args.returnError)

			pkgContent, err := FetchPackageContent(test.args.crossplanePackage)

			if len(test.expects.errorMessage) == 0 {
				if err == nil {
					require.Equal(t, test.expects.crossplanePackageContent, pkgContent)
				}
			} else {
				require.EqualError(t, err, test.expects.errorMessage)
			}
		})
	}
}

func TestSavePackage(t *testing.T) {
	type args struct {
		crossplanePackage string
		returnContent     string
		returnError       error
	}
	type expects struct {
		fileChecksum string
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
				crossplanePackage: "build-7b9b1d70/crossplane/provider-abc",
				returnContent:     csdProvider,
			},
			expects: expects{
				fileChecksum: "36632d1c51a7eea28d69ce096747fc6ca3b130569dc4b4ff5b0e512954ac77fb",
			},
		},
		{
			description: "error case",
			args: args{
				returnError: fmt.Errorf("sth went wrong"),
			},
			expects: expects{
				errorMessage: "sth went wrong",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			extractContainerImage = returnStaticXPKG(t, test.args.crossplanePackage, test.args.returnContent, test.args.returnError)

			if tmpFile, err := os.CreateTemp("", "test"); err == nil {
				defer os.Remove(tmpFile.Name())

				err = SavePackage(test.args.crossplanePackage, tmpFile.Name())

				if len(test.expects.errorMessage) == 0 {
					if err == nil {
						assertSHA256(t, tmpFile.Name(), test.expects.fileChecksum)
					}
				} else {
					require.EqualError(t, err, test.expects.errorMessage)
				}
			}
		})
	}
}

func assertSHA256(t *testing.T, file string, expectedSHA256 string) {
	if f, err := os.Open(file); err == nil {
		defer f.Close()
		hasher := sha256.New()

		if _, err := io.Copy(hasher, f); err == nil {
			require.Equal(t, expectedSHA256, hex.EncodeToString(hasher.Sum(nil)))
		}
	}
}

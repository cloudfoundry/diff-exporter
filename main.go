package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/diff-exporter/layer"
)

//type Exporter interface {
//	Export() (io.ReadCloser, error)
//}

func main() {
	outputDir, containerId, bundlePath := parseFlags()

	exporter := layer.New(containerId, bundlePath)

	// export the layer
	tarStream, err := exporter.Export()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting layer: %s", err.Error())
		os.Exit(1)
	}

	// setup intermediate tar file
	outfile, err := ioutil.TempFile(outputDir, "export-archive")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp file: %s", err.Error())
		os.Exit(1)
	}

	// read from the tar stream and calculate the shasum at the same time
	shasum := sha256.New()
	writer := io.MultiWriter(outfile, shasum)

	_, err = io.Copy(writer, tarStream)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error copying tar stream: %s", err.Error())
		os.Exit(1)
	}

	outfile.Close()
	tarStream.Close()

	// rename the intermediate tar file to be the shasum of its contents
	sha256Name := fmt.Sprintf("%x", shasum.Sum(nil))

	err = os.Rename(outfile.Name(), filepath.Join(outputDir, sha256Name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error renaming intermediate tar file: %s", err.Error())
		os.Exit(1)
	}
}

func parseFlags() (string, string, string) {
	outputDir := flag.String("outputDir", os.TempDir(), "Output directory for exported layer")
	containerId := flag.String("containerId", "", "Container ID to use")
	bundlePath := flag.String("bundlePath", "", "Bundle path to use")
	flag.Parse()

	return *outputDir, *containerId, *bundlePath
}

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

type Exporter interface {
	Export() (io.ReadCloser, error)
}

func main() {
	outputDir, containerId, bundlePath := parseFlags()

	exporter := layer.New(containerId, bundlePath)

	outfile, sha256Name, err := getTgzFile(exporter, outputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating intermediate tar file: %s", err.Error())
		os.Exit(1)
	}

	err = os.Rename(outfile.Name(), filepath.Join(outputDir, sha256Name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error renaming intermediate tar file: %s", err.Error())
		os.Exit(1)
	}
}

func getTgzFile(exporter Exporter, outputDir string) (outfile *os.File, sha256Name string, err error) {
	// export the layer
	tgzStream, err := exporter.Export()
	if err != nil {
		return outfile, sha256Name, fmt.Errorf("Error exporting layer: %s", err.Error())
	}
	defer tgzStream.Close()

	// setup intermediate tar file
	outfile, err = ioutil.TempFile(outputDir, "export-archive")
	if err != nil {
		return outfile, sha256Name, fmt.Errorf("Error creating outfile: %s", err.Error())
	}
	defer outfile.Close()

	// read from the tar stream and calculate the shasum at the same time
	shasum := sha256.New()
	writer := io.MultiWriter(outfile, shasum)

	_, err = io.Copy(writer, tgzStream)
	if err != nil {
		return outfile, sha256Name, fmt.Errorf("Error copying tar stream: %s", err.Error())
	}

	// rename the intermediate tar file to be the shasum of its contents
	return outfile, fmt.Sprintf("%x", shasum.Sum(nil)), nil
}

func parseFlags() (string, string, string) {
	outputDir := flag.String("outputDir", os.TempDir(), "Output directory for exported layer")
	containerId := flag.String("containerId", "", "Container ID to use")
	bundlePath := flag.String("bundlePath", "", "Bundle path to use")
	flag.Parse()

	return *outputDir, *containerId, *bundlePath
}

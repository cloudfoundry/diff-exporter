package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/archive/tar"
	"github.com/Microsoft/go-winio/backuptar"
	"github.com/Microsoft/hcsshim"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const whiteoutPrefix = ".wh."

func main() {
	var err error
	outputDir, containerId, bundlePath, driverStore := parseFlags()

	volumeStore := filepath.Join(driverStore, "volumes")

	driverInfo := hcsshim.DriverInfo{Flavour: 1, HomeDir: volumeStore}

	err = hcsshim.UnprepareLayer(driverInfo, containerId)
	if err != nil {
		fmt.Errorf("Error unpreparing layer: %s", err.Error())
		os.Exit(1)
	}

	// read bundle path
	f, err := ioutil.ReadFile(bundlePath)
	if err != nil {
		fmt.Errorf("Error reading bundle path: %s", err.Error())
		os.Exit(1)
	}

	var bundleSpec specs.Spec
	err = json.Unmarshal(f, &bundleSpec)
	if err != nil {
		fmt.Errorf("Error unmarshaling bundle: %s", err.Error())
		os.Exit(1)
	}

	// export the layer
	tarStream, err := exportLayer(containerId, bundleSpec.Windows.LayerFolders, driverInfo)
	if err != nil {
		fmt.Errorf("Error exporting layer: %s", err.Error())
		os.Exit(1)
	}

	// setup intermediate tar file
	outfile, err := ioutil.TempFile(outputDir, "export-archive")
	if err != nil {
		os.Exit(1)
	}
	defer outfile.Close()

	// read from the tar stream and calculate the shasum at the same time
	shasum := sha256.New()
	writer := io.MultiWriter(outfile, shasum)

	_, err = io.Copy(writer, tarStream)
	if err != nil {
		fmt.Errorf("Error copying tar stream: %s", err.Error())
	}
	tarStream.Close()

	// rename the intermediate tar file to be the shasum of its contents
	sha256Name := fmt.Sprintf("%x", shasum.Sum(nil))
	err = os.Rename(outfile.Name(), filepath.Join(outputDir, sha256Name))
	if err != nil {
		fmt.Errorf("Error renaming intermediate tar file: %s", err.Error())
	}
}

func exportLayer(cid string, parentLayerPaths []string, driverInfo hcsshim.DriverInfo) (io.ReadCloser, error) {
	archive, w := io.Pipe()
	go func() {
		err := winio.RunWithPrivilege(winio.SeBackupPrivilege, func() error {
			r, err := hcsshim.NewLayerReader(driverInfo, cid, parentLayerPaths)
			if err != nil {
				return err
			}

			err = writeTarFromLayer(r, w)
			cerr := r.Close()
			if err == nil {
				err = cerr
			}
			return err
		})
		w.CloseWithError(err)
	}()

	return archive, nil
}

func writeTarFromLayer(r hcsshim.LayerReader, w io.Writer) error {
	t := tar.NewWriter(w)
	for {
		name, size, fileInfo, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if fileInfo == nil {
			// Write a whiteout file.
			hdr := &tar.Header{
				Name: filepath.ToSlash(filepath.Join(filepath.Dir(name), whiteoutPrefix+filepath.Base(name))),
			}
			err := t.WriteHeader(hdr)
			if err != nil {
				return err
			}
		} else {
			err = backuptar.WriteTarFileFromBackupStream(t, r, name, size, fileInfo)
			if err != nil {
				return err
			}
		}
	}
	return t.Close()
}

func parseFlags() (string, string, string, string) {
	outputDir := flag.String("outputDir", os.TempDir(), "Output directory for exported layer")
	containerId := flag.String("containerId", "", "Container ID to use")
	bundlePath := flag.String("bundlePath", "", "Bundle path to use")
	driverStore := flag.String("driverStore", "", "Driver store to use")
	flag.Parse()

	return *outputDir, *containerId, *bundlePath, *driverStore
}

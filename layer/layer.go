package layer

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/archive/tar"
	"github.com/Microsoft/go-winio/backuptar"
	"github.com/Microsoft/hcsshim"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const whiteoutPrefix = ".wh."

type Exporter struct {
	containerId string
	bundlePath  string
}

func New(containerId string, bundlePath string) *Exporter {
	return &Exporter{
		containerId: containerId,
		bundlePath:  bundlePath,
	}
}

func (e *Exporter) Export() (io.ReadCloser, error) {
	// read bundle path
	content, err := ioutil.ReadFile(e.bundlePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading bundle path: %s", err.Error())
		return nil, err
	}

	// parse bundle spec
	var bundleSpec specs.Spec
	err = json.Unmarshal(content, &bundleSpec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error unmarshaling bundle: %s", err.Error())
		return nil, err
	}

	// setup driver info
	driverStore := getDriverStore(bundleSpec.Windows.LayerFolders[0])
	volumeStore := filepath.Join(driverStore, "volumes")
	driverInfo := hcsshim.DriverInfo{Flavour: 1, HomeDir: volumeStore}

	// unprepare layer
	err = hcsshim.UnprepareLayer(driverInfo, e.containerId)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error unpreparing layer: %s", err.Error())
		return nil, err
	}

	return exportLayer(e.containerId, bundleSpec.Windows.LayerFolders, driverInfo)
}

func getDriverStore(layerPath string) string {
	parts := strings.Split(layerPath, "\\")
	return strings.Join(parts[:len(parts)-2], "\\") // TODO: use filepath
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

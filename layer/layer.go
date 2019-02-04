// The exportLayer and writeTarFromLayer functions in this file are from
// github.com/Microsoft/hcsshim, licensed under MIT:
//
// The MIT License (MIT)
//
// Copyright (c) 2015 Microsoft
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package layer

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/archive/tar"
	"github.com/Microsoft/go-winio/backuptar"
	"github.com/Microsoft/hcsshim"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const (
	specConfig     = "config.json"
	whiteoutPrefix = ".wh."
)

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
	// read config.json from bundle directory
	content, err := ioutil.ReadFile(filepath.Join(e.bundlePath, specConfig))
	if err != nil {
		return nil, fmt.Errorf("Error reading bundle config.json: %s", err.Error())
	}

	// parse bundle spec
	var bundleSpec specs.Spec
	err = json.Unmarshal(content, &bundleSpec)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling bundle: %s", err.Error())
	}

	// setup driver info
	driverStore := getDriverStore(bundleSpec.Windows.LayerFolders[0])
	volumeStore := filepath.Join(driverStore, "volumes")
	driverInfo := hcsshim.DriverInfo{Flavour: 1, HomeDir: volumeStore}

	// unprepare layer
	err = hcsshim.UnprepareLayer(driverInfo, e.containerId)
	if err != nil {
		return nil, fmt.Errorf("Error unpreparing layer: %s", err.Error())
	}

	return exportLayer(e.containerId, bundleSpec.Windows.LayerFolders, driverInfo)
}

func getDriverStore(layerPath string) string {
	return filepath.Dir(filepath.Dir(layerPath))
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
	g := gzip.NewWriter(w)
	t := tar.NewWriter(g)
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
	err := t.Close()
	if err != nil {
		return err
	}
	return g.Close()
}

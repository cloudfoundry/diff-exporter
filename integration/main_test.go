package integration_test

import (
	"io/ioutil"
	"os"

	//"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

var _ = Describe("diff_exporter", func() {
	Context("when called with the correct arguments", func() {

		var (
			bundlePath  string
			containerId string
			bundleSpec  specs.Spec
			outputDir   string
			err         error
		)

		BeforeEach(func() {
			bundlePath, err = ioutil.TempDir("", "winccontainer")
			Expect(err).To(Succeed())

			containerId = filepath.Base(bundlePath)

			bundleSpec = helpers.GenerateRuntimeSpec(helpers.CreateVolume(rootfsURI, containerId))

			helpers.CreateContainer(bundleSpec, bundlePath, containerId)
			helpers.StartContainer(containerId)

			var args = []string{"powershell.exe", "new-item", "C:\\hello.txt"}

			_, _, err := helpers.ExecInContainer(containerId, args, false)
			Expect(err).To(Succeed())

			helpers.DeleteContainer(containerId)

			outputDir, err = ioutil.TempDir("", "diffoutput")
			Expect(err).To(Succeed())
		})

		AfterEach(func() {
			err = os.RemoveAll(bundlePath)
			Expect(err).To(Succeed())

			err = os.RemoveAll(outputDir)
			Expect(err).To(Succeed())

			helpers.DeleteVolume(containerId)
		})

		It("outputs a tarfile containing the result of running a command in the container", func() {

			_, _, err = helpers.Execute(exec.Command(diffBin, "-outputDir", outputDir, "-containerId", containerId, "-bundlePath", filepath.Join(bundlePath, "config.json"), "-driverStore", grootImageStore)) // TODO: read driverStore from the parent layer paths in the config
			Expect(err).ToNot(HaveOccurred())

			files, err := ioutil.ReadDir(outputDir)
			Expect(err).ToNot(HaveOccurred())

			firstFile := files[0]
			Expect(helpers.IsTarFile(filepath.Join(outputDir, firstFile.Name()))).To(BeTrue())
		})
	})
})

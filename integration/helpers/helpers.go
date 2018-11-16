package helpers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Microsoft/hcsshim"
	"github.com/h2non/filetype/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

type Helpers struct {
	wincBin         string
	grootBin        string
	grootImageStore string
	wincNetworkBin  string
	gatewayFileName string
	debug           bool
	logFile         *os.File
	windowsBuild    int
}

func NewHelpers(wincBin, grootBin, grootImageStore, wincNetworkBin string, debug bool) *Helpers {
	output, err := exec.Command("powershell", "-command", "[System.Environment]::OSVersion.Version.Build").CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	windowsBuild, err := strconv.Atoi(strings.TrimSpace(string(output)))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	h := &Helpers{
		wincBin:         wincBin,
		grootBin:        grootBin,
		grootImageStore: grootImageStore,
		wincNetworkBin:  wincNetworkBin,
		gatewayFileName: "c:\\var\\vcap\\data\\winc-network\\gateways.json",
		debug:           debug,
		windowsBuild:    windowsBuild,
	}

	if h.debug {
		var err error
		h.logFile, err = ioutil.TempFile("", "log")
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
	}
	return h
}

//func (h *Helpers) Logs() []byte {
//	h.logFile.Close()
//	content, err := ioutil.ReadFile(h.logFile.Name())
//	ExpectWithOffset(1, err).ToNot(HaveOccurred())
//	Expect(os.RemoveAll(h.logFile.Name())).To(Succeed())
//	return content
//}

//func (h *Helpers) GetContainerState(containerId string) specs.State {
//	stdOut, _, err := h.Execute(h.ExecCommand(h.wincBin, "state", containerId))
//	ExpectWithOffset(1, err).ToNot(HaveOccurred())
//
//	var state specs.State
//	ExpectWithOffset(1, json.Unmarshal(stdOut.Bytes(), &state)).To(Succeed())
//	return state
//}

func (h *Helpers) GenerateBundle(bundleSpec specs.Spec, bundlePath string) {
	ExpectWithOffset(1, os.MkdirAll(bundlePath, 0666)).To(Succeed())
	config, err := json.Marshal(&bundleSpec)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	configFile := filepath.Join(bundlePath, "config.json")
	ExpectWithOffset(1, ioutil.WriteFile(configFile, config, 0666)).To(Succeed())
}

func (h *Helpers) CreateContainer(bundleSpec specs.Spec, bundlePath, containerId string) {
	h.GenerateBundle(bundleSpec, bundlePath)
	_, _, err := h.Execute(h.ExecCommand(h.wincBin, "create", "-b", bundlePath, containerId))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

//func (h *Helpers) RunContainer(bundleSpec specs.Spec, bundlePath, containerId string) {
//	h.GenerateBundle(bundleSpec, bundlePath)
//	_, _, err := h.Execute(h.ExecCommand(h.wincBin, "run", "--detach", "-b", bundlePath, containerId))
//	ExpectWithOffset(1, err).NotTo(HaveOccurred())
//}

func (h *Helpers) StartContainer(containerId string) {
	Eventually(func() error {
		_, _, err := h.Execute(h.ExecCommand(h.wincBin, "start", containerId))
		return err
	}).Should(Succeed())
}

func (h *Helpers) DeleteContainer(id string) {
	if h.ContainerExists(id) {
		output, err := h.ExecCommand(h.wincBin, "delete", id).CombinedOutput()
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), string(output))
	}
}

func (h *Helpers) CreateVolume(rootfsURI, containerId string) specs.Spec {
	stdOut := new(bytes.Buffer)
	stdErr := new(bytes.Buffer)
	cmd := exec.Command(h.grootBin, "--driver-store", h.grootImageStore, "create", rootfsURI, containerId)
	cmd.Stdout = stdOut
	cmd.Stderr = stdErr
	ExpectWithOffset(1, cmd.Run()).To(Succeed(), fmt.Sprintf("groot stdout: %s\n\n groot stderr: %s\n\n", stdOut.String(), stdErr.String()))
	var spec specs.Spec
	ExpectWithOffset(1, json.Unmarshal(stdOut.Bytes(), &spec)).To(Succeed())
	return spec
}

func (h *Helpers) DeleteVolume(id string) {
	output, err := exec.Command(h.grootBin, "--driver-store", h.grootImageStore, "delete", id).CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), string(output))
}

func (h *Helpers) ContainerExists(containerId string) bool {
	query := hcsshim.ComputeSystemQuery{
		IDs: []string{containerId},
	}
	containers, err := hcsshim.GetContainers(query)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return len(containers) > 0
}

func (h *Helpers) ExecInContainer(id string, args []string, detach bool) (*bytes.Buffer, *bytes.Buffer, error) {
	var defaultArgs []string

	defaultArgs = []string{"exec"}
	// on 1709, need non-admin user for networking tests
	if h.windowsBuild == 16299 {
		defaultArgs = append(defaultArgs, "-u", "vcap")
	}

	if detach {
		defaultArgs = append(defaultArgs, "-d")
	}

	defaultArgs = append(defaultArgs, id)

	return h.Execute(h.ExecCommand(h.wincBin, append(defaultArgs, args...)...))
}

func (h *Helpers) GenerateRuntimeSpec(baseSpec specs.Spec) specs.Spec {
	return specs.Spec{
		Version: specs.Version,
		Process: &specs.Process{
			Args: []string{"waitfor", "ever", "/t", "9999"},
			Cwd:  "C:\\",
		},
		Root: &specs.Root{
			Path: baseSpec.Root.Path,
		},
		Windows: &specs.Windows{
			LayerFolders: baseSpec.Windows.LayerFolders,
		},
	}
}

//func (h *Helpers) RandomContainerId() string {
//	max := big.NewInt(math.MaxInt64)
//	r, err := rand.Int(rand.Reader, max)
//	ExpectWithOffset(1, err).NotTo(HaveOccurred())
//
//	return fmt.Sprintf("%d", r.Int64())
//}

//func (h *Helpers) CopyFile(dst, src string) {
//	in, err := os.Open(src)
//	ExpectWithOffset(1, err).NotTo(HaveOccurred())
//	defer in.Close()
//	out, err := os.Create(dst)
//	ExpectWithOffset(1, err).NotTo(HaveOccurred())
//	defer out.Close()
//	_, err = io.Copy(out, in)
//	closeErr := out.Close()
//	ExpectWithOffset(1, err).NotTo(HaveOccurred())
//	ExpectWithOffset(1, closeErr).NotTo(HaveOccurred())
//}

func (h *Helpers) Execute(c *exec.Cmd) (*bytes.Buffer, *bytes.Buffer, error) {
	stdOut := new(bytes.Buffer)
	stdErr := new(bytes.Buffer)
	c.Stdout = io.MultiWriter(stdOut, GinkgoWriter)
	c.Stderr = io.MultiWriter(stdErr, GinkgoWriter)
	err := c.Run()

	return stdOut, stdErr, err
}

//func (h *Helpers) ExitCode(err error) (int, error) {
//	if exiterr, ok := err.(*exec.ExitError); ok {
//		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
//			return status.ExitStatus(), nil
//		} else {
//			return -1, errors.New("Error did not have a syscall.WaitStatus")
//		}
//	} else {
//		return -1, errors.New("Error was not an exec.ExitError")
//	}
//}
//
//func (h *Helpers) TheProcessExits(containerId, image string) {
//	exited := false
//
//	for i := 0; i < 5; i++ {
//		time.Sleep(time.Duration(i) * time.Second)
//		pl := h.ContainerProcesses(containerId, image)
//		if len(pl) == 0 {
//			exited = true
//			break
//		}
//	}
//	ExpectWithOffset(1, exited).To(BeTrue())
//}
//
//func (h *Helpers) ContainerProcesses(containerId, filter string) []hcsshim.ProcessListItem {
//	container, err := hcsshim.OpenContainer(containerId)
//	Expect(err).To(Succeed())
//
//	pl, err := container.ProcessList()
//	Expect(err).To(Succeed())
//
//	if filter != "" {
//		var filteredPL []hcsshim.ProcessListItem
//		for _, v := range pl {
//			if v.ImageName == filter {
//				filteredPL = append(filteredPL, v)
//			}
//		}
//
//		return filteredPL
//	}
//
//	return pl
//}
//
func (h *Helpers) ExecCommand(command string, args ...string) *exec.Cmd {
	allArgs := []string{}
	if h.debug {
		allArgs = append([]string{"--log", h.logFile.Name(), "--debug"}, args...)
	} else {
		allArgs = args[0:]
	}
	return exec.Command(command, allArgs...)
}

func (h *Helpers) IsTarFile(path string) (bool, error) {
	file, _ := os.Open(path)
	defer file.Close()

	// We only have to pass the file header = first 261 bytes
	head := make([]byte, 262)
	file.Read(head)

	return matchers.Tar(head), nil
}

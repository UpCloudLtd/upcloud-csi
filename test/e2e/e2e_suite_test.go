//go:build e2e
// +build e2e

package e2e_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/UpCloudLtd/upcloud-csi/test/e2e/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

type cmd struct {
	command  string
	args     []string
	startLog string
	endLog   string
}

var _ = BeforeSuite(func() {
	deployCSIDriver := cmd{
		command:  "make",
		args:     []string{"deploy-csi"},
		startLog: "Installing CSI driver...",
		endLog:   "CSI Driver installed...",
	}
	execCmd([]cmd{deployCSIDriver})
})

var _ = Describe("", func() {
	kubeconfig := mock.GetKubeconfig()
	configPath := fmt.Sprintf("KUBECONFIG=%s", kubeconfig)

	It("should run e2e tests", func() {
		e2e := cmd{
			command:  "make",
			args:     []string{"deploy-csi", configPath},
			startLog: "Running e2e tests...",
			endLog:   "e2e tests successfully completed",
		}
		execCmd([]cmd{e2e})
	})
})

func execCmd(cmds []cmd) {
	err := os.Chdir("../..")
	Expect(err).NotTo(HaveOccurred())

	defer func() {
		err := os.Chdir("test/e2e")
		Expect(err).NotTo(HaveOccurred())
	}()

	projectRoot, err := os.Getwd()

	Expect(err).NotTo(HaveOccurred())
	Expect(strings.HasSuffix(projectRoot, "upcloud-csi")).To(Equal(true))

	for _, cmd := range cmds {
		log.Println(cmd.startLog)
		cmdSh := exec.Command(cmd.command, cmd.args...)
		cmdSh.Dir = projectRoot
		cmdSh.Stdout = os.Stdout
		cmdSh.Stderr = os.Stderr
		err = cmdSh.Run()

		Expect(err).NotTo(HaveOccurred())
		log.Println(cmd.endLog)
	}
}

var _ = AfterSuite(func() {
	kubeconfig := mock.GetKubeconfig()
	configPath := fmt.Sprintf("KUBECONFIG=%s", kubeconfig)

	// TearDown
	cleanTests := cmd{
		command:  "make",
		args:     []string{"clean-tests", configPath},
		startLog: "cleaning test environment...",
		endLog:   "test environment cleaned...",
	}

	execCmd([]cmd{cleanTests})
})

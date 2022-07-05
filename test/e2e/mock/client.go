package mock

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type ExecParams struct {
	Command        string
	ExpectedString string
	PodName        string
}

func NewClient(namespace string) (*Client, error) {
	kubeconfig := GetKubeconfig()

	config, err := clientcmd.BuildConfigFromFlags(
		"",
		kubeconfig,
	)
	if err != nil {
		return nil, err
	}

	k8s, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Client{k8s: k8s, ns: namespace}, nil
}

func (c *Client) Exec(params ExecParams) error {
	err := os.Chdir("../..")
	if err != nil {
		return err
	}

	defer func() {
		err := os.Chdir("test/e2e")
		Expect(err).NotTo(HaveOccurred())
	}()

	projectRoot, err := os.Getwd()
	if err != nil {
		return err
	}

	if !strings.HasSuffix(projectRoot, "upcloud-csi") {
		return fmt.Errorf("project root must be upcloud-csi")
	}

	cmd := "kubectl"
	args := []string{"exec", "-i", params.PodName, "--", "/bin/sh", "-c", "cat ./temp"}

	cmdSh := exec.Command(cmd, args...)
	cmdSh.Dir = projectRoot
	cmdSh.Stdout = os.Stdout
	cmdSh.Stderr = os.Stderr

	kubeconfig := GetKubeconfig()
	cmdSh.Env = append(os.Environ(), kubeconfig)

	log.Println("executing command")
	err = cmdSh.Run()

	return err
}

func GetKubeconfig() string {
	if os.Getenv("KUBECONFIG") == "" {
		return filepath.Join(homedir.HomeDir(), ".kube", "config")
	} else {
		return os.Getenv("KUBECONFIG")
	}
}

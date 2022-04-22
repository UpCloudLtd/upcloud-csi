package testruns

import (
	"context"
	"github.com/UpCloudLtd/upcloud-csi/test/e2e/mock"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	"log"
)

func TestProvisionResizeVolume() {
	ctx := context.Background()
	client, err := mock.NewClient("default")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	pvcName := uuid.New().String()
	pvc, err := client.CreatePVC(ctx, pvcName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	podName := uuid.New().String()
	pod, err := client.CreatePod(ctx, podName, pvcName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("waiting for pod to be ready")
	err = client.WaitForPod(ctx, pod.Name, pod.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	_, err = client.ResizePVC(ctx, pvc.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

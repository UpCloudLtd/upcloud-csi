package testruns

import (
	"context"
	"log"

	"github.com/UpCloudLtd/upcloud-csi/test/e2e/mock"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
)

// TODO: refactor into smaller files

func TestPublishUnPublishVolume() {
	ctx := context.Background()
	client, err := mock.NewClient("default")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	pvcName := uuid.New().String()
	pvc, err := client.CreatePVC(ctx, pvcName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("waiting for pvc to be bound")
	err = client.WaitForPVC(ctx, pvc.Name, pvc.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("creating pod...")
	podName := uuid.New().String()
	pod, err := client.CreatePod(ctx, podName, pvcName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("waiting for pod to be ready")
	err = client.WaitForPod(ctx, pod.Name, pod.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = client.DeletePod(ctx, pod.Name, pod.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

package testruns

import (
	"context"
	"github.com/UpCloudLtd/upcloud-csi/test/e2e/mock"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	"log"
)

func TestListVolumes() {
	ctx := context.Background()
	client, err := mock.NewClient("default")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	pvcName := uuid.New().String()
	pvc, err := client.CreatePVC(ctx, pvcName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Print("waiting for pvc to be bound")
	err = client.WaitForPVC(ctx, pvc.Name, pvc.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	volumes, err := client.ListVolumes(ctx)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(len(volumes.Items)).NotTo(gomega.BeZero())
}

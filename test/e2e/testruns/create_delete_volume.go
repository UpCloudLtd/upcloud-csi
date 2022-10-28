package testruns

import (
	"context"
	"log"

	"github.com/UpCloudLtd/upcloud-csi/test/e2e/mock"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
)

func TestCreateDeleteVolume() {
	ctx := context.Background()
	client, err := mock.NewClient("default")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	pvcName := uuid.New().String()
	pvc, err := client.CreatePVC(ctx, pvcName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	log.Println("pvc created")

	log.Println("waiting for pvc to be bound")
	err = client.WaitForPVC(ctx, pvc.Name, pvc.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = client.DeletePVC(ctx, pvc.Name, pvc.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("pvc deleted")
}

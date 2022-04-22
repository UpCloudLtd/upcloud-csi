package testruns

import (
	"context"
	"github.com/UpCloudLtd/upcloud-csi/test/e2e/mock"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	"log"
)

func TestDataPersistenceDeployment() {
	ctx := context.Background()
	client, err := mock.NewClient("default")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	pvcName := uuid.New().String()
	pvc, err := client.CreatePVC(ctx, pvcName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("waiting for pvc to be bound")
	err = client.WaitForPVC(ctx, pvc.Name, pvc.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("creating a deployment...")
	deployment, err := client.CreateDeployment(ctx, pvc, "echo 'hello world' >> ./temp; sleep 1000")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("waiting for deployment to be ready")
	err = client.WaitForDeployment(ctx, deployment.Name, deployment.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("replacing pod...")
	err = client.ReplaceDeploymentPod(ctx)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	log.Println("waiting for deployment to be ready")
	err = client.WaitForDeployment(ctx, deployment.Name, deployment.Namespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	pod, err := client.GetPod()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = client.Exec(mock.ExecParams{
		Command:        "cat ./temp",
		PodName:        pod.Name,
		ExpectedString: "hello world",
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = client.DeleteDeployment(ctx, deployment.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

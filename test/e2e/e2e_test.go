package e2e

import (
	"github.com/UpCloudLtd/upcloud-csi/test/e2e/testruns"
	. "github.com/onsi/ginkgo/v2"
	"log"
)

var _ = Describe("", func() {
	It("Resize Volume", func() {
		testruns.TestProvisionResizeVolume()
	})
	It("Create Delete Volume", func() {
		testruns.TestCreateDeleteVolume()
	})
	It("List Volumes", func() {
		testruns.TestListVolumes()
	})
	It("Attach Detach Volume", func() {
		testruns.TestDataPersistenceDeployment()
		log.Println("Persistence Passed")
	})

})

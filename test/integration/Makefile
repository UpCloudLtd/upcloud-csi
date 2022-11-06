.PHONY: test 
test: check-test-env
	go test -timeout 1h -v ./... --ginkgo.fail-fast

.PHONY: check-test-env
check-test-env:
ifndef UPCLOUD_TEST_USERNAME
	$(error UPCLOUD_TEST_USERNAME is undefined)
endif
ifndef UPCLOUD_TEST_PASSWORD
	$(error UPCLOUD_TEST_PASSWORD is undefined)
endif
ifndef UPCLOUD_TEST_HOSTNAME
	$(error UPCLOUD_TEST_HOSTNAME is undefined)
endif

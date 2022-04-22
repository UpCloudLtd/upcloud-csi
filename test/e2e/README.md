# Running End-to-End Tests

## Setup

Make sure that your desired `KUBECONFIG` location is available in your env. The default behavior is to look for a 
`KUBECONFIG` in your `~/.kube/config` file. (Make sure to use absolute paths without `../` as the tests will change 
directories)

## Running

You can run the entire e2e suite from the `test/e2e` directory by running:

```
ginkgo
```

You can also run the tests manually inside your IDE. Individual tests can be found in `test/e2e/testruns`. 
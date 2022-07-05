package mock

import (
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

var storageClassName = "upcloud-block-storage"

type Client struct {
	k8s kubernetes.Interface
	ns  string
}

func (c *Client) CreatePod(ctx context.Context, podName, pvcName string) (*v1.Pod, error) {
	req := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					Name:    "main",
					Image:   "busybox",
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", "echo 'hello world' >> ./temp; sleep 1000"},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      pvcName,
							MountPath: "/data",
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: pvcName,
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}

	pod, err := c.k8s.CoreV1().Pods(c.ns).Create(ctx, req, metav1.CreateOptions{})

	return pod, err
}

func (c *Client) DeletePod(ctx context.Context, podName, namespace string) error {
	return c.k8s.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
}

func (c *Client) isPodRunning(ctx context.Context, podName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.k8s.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return pod.Status.Phase == v1.PodRunning, nil
	}
}

func (c *Client) WaitForPod(ctx context.Context, podName, namespace string) error {
	return wait.PollImmediate(time.Second, time.Minute, c.isPodRunning(ctx, podName, namespace))
}

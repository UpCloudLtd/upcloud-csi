package mock

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (c *Client) CreateDeployment(ctx context.Context, pvc *v1.PersistentVolumeClaim, command string) (*appv1.Deployment, error) {
	replicaCount := int32(1)
	generateName := "csi-volume-tester-"
	randInt, err := rand.Int(rand.Reader, big.NewInt(999))
	if err != nil {
		return &appv1.Deployment{}, err
	}
	selectorValue := fmt.Sprintf("%s%d", generateName, randInt)

	deployment := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName,
		},
		Spec: appv1.DeploymentSpec{
			Replicas: &replicaCount,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": selectorValue},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": selectorValue},
				},
				Spec: v1.PodSpec{
					NodeSelector: map[string]string{"kubernetes.io/os": "linux"},
					Containers: []v1.Container{
						{
							Name:    "main",
							Image:   "busybox",
							Command: []string{"/bin/sh"},
							Args:    []string{"-c", command},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      pvc.Name,
									MountPath: "/data",
								},
							},
						},
					},
					RestartPolicy: v1.RestartPolicyAlways,
					Volumes: []v1.Volume{
						{
							Name: pvc.Name,
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvc.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	return c.k8s.AppsV1().Deployments(c.ns).Create(ctx, deployment, metav1.CreateOptions{})
}

func (c *Client) ReplaceDeploymentPod(ctx context.Context) error {
	p, err := c.k8s.CoreV1().Pods(c.ns).List(ctx, metav1.ListOptions{})
	if err != nil || len(p.Items) != 1 {
		return err
	}

	err = c.k8s.CoreV1().Pods(c.ns).Delete(ctx, p.Items[0].Name, metav1.DeleteOptions{})

	return err
}

func (c *Client) GetPod() (*v1.Pod, error) {
	var pod v1.Pod
	podList, err := c.k8s.CoreV1().Pods(c.ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, p := range podList.Items {
		if p.Status.Phase == v1.PodRunning {
			pod = p
			break
		}
	}
	if pod.Name == "" {
		return nil, fmt.Errorf("running pod not found")
	}

	return &pod, nil
}

func (c *Client) DeleteDeployment(ctx context.Context, deploymentName string) error {
	return c.k8s.AppsV1().Deployments(c.ns).Delete(ctx, deploymentName, metav1.DeleteOptions{})
}

func (c *Client) WaitForDeployment(ctx context.Context, deploymentName, namespace string) error {
	return wait.PollImmediate(time.Second, time.Minute, c.isDeploymentRunning(ctx, deploymentName, namespace))
}

func (c *Client) isDeploymentRunning(ctx context.Context, deploymentName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		deployment, err := c.k8s.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return deployment.Status.ReadyReplicas == 1, nil
	}
}

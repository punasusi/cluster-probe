package nettest

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	testNamespace  = "cluster-probe-nettest"
	testPodPrefix  = "nettest-"
	testImage      = "busybox:1.36"
	testListenPort = 8080
)

type TestPod struct {
	Name     string
	NodeName string
	PodIP    string
}

func (n *NetworkTest) EnsureNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "cluster-probe",
				"app.kubernetes.io/component": "network-test",
			},
		},
	}

	_, err := n.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	return nil
}

func (n *NetworkTest) CreateTestPods(ctx context.Context, nodes []corev1.Node) ([]TestPod, error) {
	var testPods []TestPod

	for _, node := range nodes {
		podName := testPodPrefix + sanitizeNodeName(node.Name)

		if n.verbose {
			fmt.Fprintf(os.Stderr, "[network-test] Creating test pod on node %s...\n", node.Name)
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: testNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":      "cluster-probe",
					"app.kubernetes.io/component": "network-test",
					"cluster-probe/node":          node.Name,
				},
			},
			Spec: corev1.PodSpec{
				NodeName:      node.Name,
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:    "nettest",
						Image:   testImage,
						Command: []string{"sleep", "3600"},
					},
				},
				Tolerations: []corev1.Toleration{
					{
						Operator: corev1.TolerationOpExists,
					},
				},
			},
		}

		_, err := n.client.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			if errors.IsAlreadyExists(err) {
				if n.verbose {
					fmt.Fprintf(os.Stderr, "[network-test] Pod %s already exists, reusing\n", podName)
				}
			} else {
				return nil, fmt.Errorf("failed to create pod on node %s: %w", node.Name, err)
			}
		}

		testPods = append(testPods, TestPod{
			Name:     podName,
			NodeName: node.Name,
		})
	}

	return testPods, nil
}

func (n *NetworkTest) WaitForPodsReady(ctx context.Context, pods []TestPod, timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for i := range pods {
		pod := &pods[i]

		err := wait.PollUntilContextCancel(timeoutCtx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
			p, err := n.client.CoreV1().Pods(testNamespace).Get(ctx, pod.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}

			if p.Status.Phase != corev1.PodRunning {
				return false, nil
			}

			for _, cs := range p.Status.ContainerStatuses {
				if !cs.Ready {
					return false, nil
				}
			}

			pod.PodIP = p.Status.PodIP
			if n.verbose {
				fmt.Fprintf(os.Stderr, "[network-test] Pod %s ready (%s)\n", pod.Name, pod.PodIP)
			}
			return true, nil
		})

		if err != nil {
			return fmt.Errorf("pod %s failed to become ready: %w", pod.Name, err)
		}
	}

	return nil
}

func (n *NetworkTest) CleanupTestPods(ctx context.Context) error {
	propagation := metav1.DeletePropagationForeground
	err := n.client.CoreV1().Namespaces().Delete(ctx, testNamespace, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	if err == nil {
		wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := n.client.CoreV1().Namespaces().Get(ctx, testNamespace, metav1.GetOptions{})
			return errors.IsNotFound(err), nil
		})
	}

	return nil
}

func (n *NetworkTest) StartListeners(ctx context.Context, pods []TestPod) {
	for _, pod := range pods {
		cmd := []string{"sh", "-c", fmt.Sprintf("nc -l -p %d &", testListenPort)}
		_, _, _ = n.ExecInPod(ctx, pod.Name, testNamespace, cmd)
	}
	time.Sleep(time.Second)
}

func sanitizeNodeName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32)
		} else {
			result = append(result, '-')
		}
	}
	if len(result) > 50 {
		result = result[:50]
	}
	return string(result)
}

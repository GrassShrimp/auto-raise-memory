package main

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})

	for _, pod := range pods.Items {
		oomContainers := getOOMContainers(pod)
		if len(oomContainers) == 0 {
			continue
		}
		switch owner := pod.OwnerReferences[0]; owner.Kind {
		case "ReplicaSet":
			rs, err := clientset.AppsV1().ReplicaSets(pod.Namespace).Get(context.TODO(), owner.Name, metav1.GetOptions{})

			if err != nil {
				panic(err.Error())
			}
			deployment, err := clientset.AppsV1().Deployments(pod.Namespace).Get(context.TODO(), rs.OwnerReferences[0].Name, metav1.GetOptions{})

			if err != nil {
				panic(err.Error())
			}

			for index, container := range deployment.Spec.Template.Spec.Containers {
				if !inSlice(container.Image, oomContainers) {
					continue
				}

				currentMemory, _ := container.Resources.Limits.Memory().AsInt64()

				addtionalMemory, _ := resource.NewQuantity(250*1024*1024, resource.BinarySI).AsInt64()

				container.Resources.Limits = v1.ResourceList{
					"memory": resource.MustParse(strconv.FormatInt(currentMemory+addtionalMemory, 10)),
				}

				deployment.Spec.Template.Spec.Containers[index] = container
			}

			clientset.AppsV1().Deployments(deployment.Namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
			fmt.Printf("Update Deployment %s at namespace %s", deployment.Name, deployment.Namespace)
		default:
			continue
		}
	}
}

func getOOMContainers(pod v1.Pod) []string {
	var containers []string
	for _, containerStatus := range pod.Status.ContainerStatuses {
		terminated := containerStatus.LastTerminationState.Terminated
		if !reflect.ValueOf(terminated).IsNil() && terminated.Reason == "OOMKilled" {
			containers = append(containers, containerStatus.Image)
		}
	}
	return containers
}

func inSlice(search string, slice []string) bool {
	for _, item := range slice {
		if len(strings.Split(search, ":")) < 2 {
			search += ":latest"
		}
		if search == item {
			return true
		}
	}
	return false
}

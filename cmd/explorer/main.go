package main

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	config, _ := rest.InClusterConfig()
	clientset, _ := kubernetes.NewForConfig(config)

	mode := "status"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	if mode == "events" {
		monitorEvents(clientset)
	} else {
		showStatus(clientset)
	}
}

func monitorEvents(c *kubernetes.Clientset) {
	fmt.Println(">>> Мониторинг событий кластера (staging)...")
	watcher, _ := c.CoreV1().Events("").Watch(context.TODO(), metav1.ListOptions{})
	for event := range watcher.ResultChan() {
		e := event.Object.(*corev1.Event)
		fmt.Printf("[%s] %s: %s\n", e.LastTimestamp.Format("15:04:05"), e.InvolvedObject.Name, e.Message)
	}
}

func showStatus(c *kubernetes.Clientset) {
	for {
		pods, _ := c.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		fmt.Printf("[%s] Найдено подов: %d\n", time.Now().Format("15:04:05"), len(pods.Items))
		time.Sleep(10 * time.Second)
	}
}

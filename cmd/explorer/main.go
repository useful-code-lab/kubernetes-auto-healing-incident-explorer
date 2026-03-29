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
	// Используем Fprintln в Stderr для мгновенного вывода в kubectl logs
	fmt.Fprintln(os.Stderr, "--- ПРИЛОЖЕНИЕ СТАРТУЕТ ---")

	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ОШИБКА КОНФИГА: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ОШИБКА КЛИЕНТА: %v\n", err)
		os.Exit(1)
	}

	mode := "status"
	if len(os.Args) > 1 {
		mode = os.Args[1] // Берем ПЕРВЫЙ аргумент
	}
	fmt.Fprintf(os.Stderr, "ВЫБРАН РЕЖИМ: [%s]\n", mode)

	if mode == "events" {
		monitorEvents(clientset)
	} else {
		showStatus(clientset)
	}
}

func monitorEvents(c *kubernetes.Clientset) {
	fmt.Fprintln(os.Stderr, ">>> Запуск Watcher...")
	// Пустая строка "" означает все неймспейсы
	watcher, err := c.CoreV1().Events("").Watch(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ОШИБКА WATCH: %v\n", err)
		return
	}

	fmt.Fprintln(os.Stderr, ">>> Соединение установлено. Жду событий...")

	for event := range watcher.ResultChan() {
		e, ok := event.Object.(*corev1.Event)
		if !ok {
			continue
		}
		// Печатаем событие
		fmt.Fprintf(os.Stdout, "[%s] %s/%s: %s\n",
			e.LastTimestamp.Format("15:04:05"),
			e.InvolvedObject.Kind,
			e.InvolvedObject.Name,
			e.Message,
		)
	}
}

func showStatus(c *kubernetes.Clientset) {
	for {
		pods, err := c.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Ошибка списка подов: %v\n", err)
		} else {
			fmt.Fprintf(os.Stdout, "[%s] Подов в кластере: %d\n", time.Now().Format("15:04:05"), len(pods.Items))
		}
		time.Sleep(10 * time.Second)
	}
}

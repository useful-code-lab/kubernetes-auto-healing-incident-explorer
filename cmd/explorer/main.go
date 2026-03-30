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
	fmt.Fprintln(os.Stderr, ">>> Запуск расширенного Health-чекера...")
	for {
		pods, err := c.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Ошибка списка подов: %v\n", err)
		} else {
			fmt.Fprintf(os.Stdout, "\n--- ОТЧЕТ ПО КЛАСТЕРУ [%s] ---\n", time.Now().Format("15:04:05"))
			fmt.Fprintf(os.Stdout, "Всего подов: %d\n", len(pods.Items))

			for _, pod := range pods.Items {
				for _, status := range pod.Status.ContainerStatuses {
					// 1. Проверка на рестарты + вывод логов
					if status.RestartCount > 0 {
						fmt.Fprintf(os.Stdout, "⚠️  Warning: [%s/%s] рестартов: %d\n",
							pod.Namespace, pod.Name, status.RestartCount)
						
						// Берем логи только если под "падал"
						logs := getPodLogs(c, pod.Namespace, pod.Name)
						fmt.Fprintf(os.Stdout, "--- ПОСЛЕДНИЕ ЛОГИ %s ---\n%s\n-------------------\n", 
							pod.Name, logs)
					}

					// 2. Проверка зависших состояний (Waiting)
					if status.State.Waiting != nil {
						reason := status.State.Waiting.Reason
						if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
							fmt.Fprintf(os.Stdout, "🚨 CRITICAL: [%s/%s] Ошибка: %s\n",
								pod.Namespace, pod.Name, reason)
						}
					}
				}
			}
		}
		time.Sleep(10 * time.Second)
	}
}

// Вспомогательная функция для логов
func getPodLogs(c *kubernetes.Clientset, namespace, name string) string {
	limit := int64(5) // последние 5 строк
	opts := &corev1.PodLogOptions{
		TailLines: &limit,
		// Можно добавить Previous: true, чтобы видеть логи именно ПЕРЕД падением
	}
	
	req := c.CoreV1().Pods(namespace).GetLogs(name, opts)
	podLogs, err := req.DoRaw(context.TODO())
	if err != nil {
		return "Логи пока недоступны (под в процессе запуска/ошибки)"
	}
	return string(podLogs)
}

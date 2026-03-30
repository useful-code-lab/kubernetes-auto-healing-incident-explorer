package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Храним количество рестартов: "namespace/name" -> count
var restartHistory = make(map[string]int32)


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

// analyzeLogText ищет ключевые слова в тексте лога
func analyzeLogText(logs string) string {
	lowerLogs := strings.ToLower(logs)
	criticalWords := []string{"error", "panic", "fatal", "fail", "exception"}

	for _, word := range criticalWords {
		if strings.Contains(lowerLogs, word) {
			return "🔥 ОБНАРУЖЕНА КРИТИЧЕСКАЯ ОШИБКА В ТЕКСТЕ ЛОГОВ!"
		}
	}
	return ""
}

func showStatus(c *kubernetes.Clientset) {
	fmt.Fprintln(os.Stderr, ">>> Мониторинг запущен. Ожидаю новых событий...")
	isFirstRun := true 

	for {
		pods, err := c.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Ошибка: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, pod := range pods.Items {
			podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
			for _, status := range pod.Status.ContainerStatuses {
				
				// 1. Инициализация (Холодный старт)
				if isFirstRun {
					restartHistory[podKey] = status.RestartCount
					continue
				}

				// 2. Детекция НОВОГО рестарта
				if status.RestartCount > restartHistory[podKey] {
					fmt.Fprintf(os.Stdout, "\n🔥 СВЕЖИЙ ИНЦИДЕНТ! [%s]\n", podKey)
					
					logs := getPodLogs(c, pod.Namespace, pod.Name)
					
					// 3. ИСПОЛЬЗУЕМ analyzeLogText (тот самый пункт 3)
					alert := analyzeLogText(logs)
					if alert != "" {
						fmt.Fprintln(os.Stdout, alert) // Печатаем красную метку, если нашли "error"
					}

					fmt.Fprintf(os.Stdout, "--- LOGS ---\n%s\n------------\n", logs)
					restartHistory[podKey] = status.RestartCount
				}
			}
		}

		if isFirstRun {
			fmt.Fprintln(os.Stderr, ">>> Базовое состояние сохранено. Тишина...")
			isFirstRun = false
		}
		time.Sleep(5 * time.Second)
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

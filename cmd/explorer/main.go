package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	// Храним количество рестартов: "namespace/name" -> count
	restartHistory = make(map[string]int32)
	totalIncidents int                       // Всего пойманных ошибок с момента старта
	startTime      = time.Now()              // Время запуска для расчета аптайма
	statusHistory  = make(map[string]string) // "namespace/name" -> "LastReason"
)

func startMetricsServer() {
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Формат Prometheus: имя_метрики{ярлыки} значение
		fmt.Fprintf(w, "# HELP explorer_incidents_total Total number of detected incidents\n")
		fmt.Fprintf(w, "# TYPE explorer_incidents_total counter\n")
		fmt.Fprintf(w, "explorer_incidents_total %d\n", totalIncidents)

		uptime := time.Since(startTime).Seconds()
		fmt.Fprintf(w, "# HELP explorer_uptime_seconds Uptime of the explorer in seconds\n")
		fmt.Fprintf(w, "# TYPE explorer_uptime_seconds gauge\n")
		fmt.Fprintf(w, "explorer_uptime_seconds %.0f\n", uptime)
	})

	fmt.Fprintln(os.Stderr, "🌐 Метрики доступны на :8080/metrics")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка сервера метрик: %v\n", err)
	}
}

func main() {
	// Используем Fprintln в Stderr для мгновенного вывода в kubectl logs
	fmt.Fprintln(os.Stderr, "--- ПРИЛОЖЕНИЕ СТАРТУЕТ ---")

	// ЗАПУСК СЕРВЕРА МЕТРИК
	go startMetricsServer()

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
	loopCounter := 0

	for {
		loopCounter++
		pods, err := c.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Ошибка списка: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, pod := range pods.Items {
			podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

			for _, status := range pod.Status.ContainerStatuses {

				// 1. Холодный старт
				if isFirstRun {
					if status.RestartCount > 0 {
						fmt.Fprintf(os.Stdout, "ℹ️  На старте: [%s] уже имеет %d рестартов\n", podKey, status.RestartCount)
					}
					restartHistory[podKey] = status.RestartCount
					continue
				}

				// 2. Улучшенная детекция ЗАВИСШИХ статусов (с памятью)
				if status.State.Waiting != nil {
					reason := status.State.Waiting.Reason
					if reason == "ImagePullBackOff" || reason == "ErrImagePull" || reason == "CreateContainerConfigError" {
						// Печатаем только если статус изменился или мы о нем еще не знали
						if statusHistory[podKey] != reason {
							fmt.Fprintf(os.Stdout, "\n\033[31m🚨 НОВЫЙ КРИТИЧЕСКИЙ СТАТУС: [%s] -> %s\033[0m\n", podKey, reason)
							statusHistory[podKey] = reason
						}
					}
				} else {
					// Если под вышел из режима ожидания (заработал), удаляем из памяти
					delete(statusHistory, podKey)
				}

				// 3. Свежий рестарт и Self-Healing
				if status.RestartCount > restartHistory[podKey] {
					totalIncidents++
					fmt.Fprintf(os.Stdout, "\n\033[33m🔥 СВЕЖИЙ ИНЦИДЕНТ #%d! [%s]\033[0m\n", totalIncidents, podKey)

					logs := getPodLogs(c, pod.Namespace, pod.Name)
					if alert := analyzeLogText(logs); alert != "" {
						fmt.Fprintf(os.Stdout, "\033[31m%s\033[0m\n", alert)
					}
					fmt.Fprintf(os.Stdout, "--- LOGS ---\n%s\n------------\n", logs)

					// Self-Healing
					if status.RestartCount > 10 && pod.Namespace != "kube-system" {
						fmt.Fprintf(os.Stdout, "🧹 \033[35mSELF-HEALING: Удаляю %s...\033[0m\n", podKey)
						c.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
						delete(restartHistory, podKey)
						continue
					}
					restartHistory[podKey] = status.RestartCount
				}
			}
		}

		if isFirstRun {
			fmt.Fprintln(os.Stderr, ">>> Базовое состояние сохранено. Вхожу в режим слежения...")
			isFirstRun = false
		}

		if loopCounter%12 == 0 {
			uptime := time.Since(startTime).Round(time.Second)
			fmt.Fprintf(os.Stdout, "\n\033[32m📊 СТАТИСТИКА: Аптайм: %v | Инцидентов: %d | Подов: %d\033[0m\n",
				uptime, totalIncidents, len(pods.Items))
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

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

var (
	// Храним количество рестартов: "namespace/name" -> count
	restartHistory = make(map[string]int32)
	totalIncidents int                       // Всего пойманных ошибок с момента старта
	startTime      = time.Now()              // Время запуска для расчета аптайма
	statusHistory  = make(map[string]string) // "namespace/name" -> "LastReason"
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
	loopCounter := 0 // Для периодических отчетов

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

				// 2. Зависшие статусы
				// 2. Детекция ЗАВИСШИХ статусов
				if status.State.Waiting != nil && !isFirstRun {
					reason := status.State.Waiting.Reason
					if reason == "ImagePullBackOff" || reason == "ErrImagePull" || reason == "CreateContainerConfigError" {
						// ПРОВЕРКА: Если мы об этом еще не писали ИЛИ статус изменился
						if statusHistory[podKey] != reason {
							fmt.Fprintf(os.Stdout, "\n🚨 НОВЫЙ КРИТИЧЕСКИЙ СТАТУС: [%s] -> %s\n", podKey, reason)
							statusHistory[podKey] = reason // Запоминаем, что уже ругались
						}
					}
				} else {
					// Если под вышел из состояния Waiting (стал Running), очищаем историю статуса
					delete(statusHistory, podKey)
				}

				// 3. Свежий рестарт
				if status.RestartCount > restartHistory[podKey] {
					totalIncidents++ // Считаем инцидент

					fmt.Fprintf(os.Stdout, "\n🔥 СВЕЖИЙ ИНЦИДЕНТ #%d! [%s]\n", totalIncidents, podKey)
					fmt.Fprintf(os.Stdout, "Рестартов: %d -> %d\n", restartHistory[podKey], status.RestartCount)

					logs := getPodLogs(c, pod.Namespace, pod.Name)
					if alert := analyzeLogText(logs); alert != "" {
						fmt.Fprintln(os.Stdout, alert)
					}
					fmt.Fprintf(os.Stdout, "--- LOGS ---\n%s\n------------\n", logs)

					// 4. Self-Healing
					if status.RestartCount > 10 && pod.Namespace != "kube-system" {
						fmt.Fprintf(os.Stdout, "🧹 SELF-HEALING: Под %s падает слишком часто. Удаляю...\n", podKey)
						err := c.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
						if err != nil {
							fmt.Fprintf(os.Stderr, "❌ Ошибка удаления: %v\n", err)
						} else {
							fmt.Fprintln(os.Stdout, "✅ Под удален для пересоздания.")
							delete(restartHistory, podKey)
							continue
						}
					}
					restartHistory[podKey] = status.RestartCount
				}
			}
		}

		if isFirstRun {
			fmt.Fprintln(os.Stderr, ">>> Базовое состояние сохранено. Вхожу в режим слежения...")
			isFirstRun = false
		}

		// 5. ПЕРИОДИЧЕСКАЯ СВОДКА (раз в 12 циклов ~ 1 минута)
		if loopCounter%12 == 0 {
			uptime := time.Since(startTime).Round(time.Second)
			fmt.Fprintf(os.Stdout, "\n📊 СТАТИСТИКА: Аптайм: %v | Всего инцидентов: %d | Активных подов: %d\n",
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

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
	totalIncidents int                           // Всего пойманных ошибок с момента старта
	startTime          = time.Now()              // Время запуска для расчета аптайма
	statusHistory      = make(map[string]string) // "namespace/name" -> "LastReason"
	lastScaleTime      = make(map[string]time.Time)

	maxReplicas   int32 = 10
	scaleCooldown       = 30 * time.Second
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

func scaleUpDeployment(c *kubernetes.Clientset, namespace, podName string) {
	fmt.Fprintf(os.Stdout, "🔍 DEBUG: Начинаю поиск владельца для пода [%s]\n", podName)

	pod, err := c.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Не нашел под: %v\n", err)
		return
	}

	for _, owner := range pod.OwnerReferences {
		if owner.Kind != "ReplicaSet" {
			continue
		}

		rs, err := c.AppsV1().ReplicaSets(namespace).Get(context.TODO(), owner.Name, metav1.GetOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Не нашел ReplicaSet: %v\n", err)
			continue
		}

		for _, rsOwner := range rs.OwnerReferences {
			if rsOwner.Kind != "Deployment" {
				continue
			}

			deploy, err := c.AppsV1().Deployments(namespace).Get(context.TODO(), rsOwner.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Ошибка доступа к Deployment: %v\n", err)
				continue
			}

			name := deploy.Name
			current := *deploy.Spec.Replicas
			ready := deploy.Status.ReadyReplicas

			fmt.Fprintf(os.Stdout, "📊 Deployment [%s]: %d/%d ready\n", name, ready, current)

			// 🚫 1. Проверка лимита
			if current >= maxReplicas {
				fmt.Fprintf(os.Stdout, "⛔ Достигнут лимит реплик (%d)\n", maxReplicas)
				return
			}

			// 🚫 2. Cooldown (анти-спам scaling)
			if last, ok := lastScaleTime[name]; ok {
				if time.Since(last) < scaleCooldown {
					fmt.Fprintf(os.Stdout, "⏳ Пропуск scaling — cooldown активен\n")
					return
				}
			}

			// 🚫 3. Проверка стабильности
			if ready < current {
				fmt.Fprintf(os.Stdout, "⚠️ Пропуск scaling — поды не готовы (%d/%d)\n", ready, current)
				return
			}

			// ✅ 4. Масштабирование
			newReplicas := current + 1
			deploy.Spec.Replicas = &newReplicas

			_, err = c.AppsV1().Deployments(namespace).Update(context.TODO(), deploy, metav1.UpdateOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Ошибка UPDATE: %v\n", err)
				return
			}

			lastScaleTime[name] = time.Now()

			fmt.Fprintf(os.Stdout, "🚀 Scaling: %s увеличен до %d реплик\n", name, newReplicas)
		}
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
	lower := strings.ToLower(logs)

	switch {
	case strings.Contains(lower, "out of memory"):
		return "💥 OOMKilled — не хватает памяти"
	case strings.Contains(lower, "connection refused"):
		return "🌐 Проблема с сетью/сервисом"
	case strings.Contains(lower, "panic"):
		return "🔥 PANIC в приложении"
	case strings.Contains(lower, "permission denied"):
		return "🔐 Проблема с правами доступа"
	}

	return ""
}

func printIndented(text string) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			fmt.Printf("  %s\n", line)
		}
	}
}

func printIncidentBlock(
	pod corev1.Pod,
	status corev1.ContainerStatus,
	prevRestarts int32,
	logs string,
	events string,
	analysis string,
) {
	fmt.Println("\n═══════════════════════════════════════════════")

	fmt.Printf("🔥 INCIDENT\n")
	fmt.Printf("Pod:        %s/%s\n", pod.Namespace, pod.Name)
	fmt.Printf("Container:  %s\n", status.Name)
	fmt.Printf("Time:       %s\n", time.Now().Format("15:04:05"))

	fmt.Println("═══════════════════════════════════════════════")

	// STATUS
	fmt.Println("\n📊 STATUS")
	fmt.Printf("  Restarts:        %d → %d\n", prevRestarts, status.RestartCount)
	fmt.Printf("  Phase:           %s\n", pod.Status.Phase)
	fmt.Printf("  Node:            %s\n", pod.Spec.NodeName)

	// 🔥 LAST TERMINATION
	if status.LastTerminationState.Terminated != nil {
		last := status.LastTerminationState.Terminated

		fmt.Fprintf(os.Stdout, "\n💀 LAST TERMINATION\n")
		fmt.Fprintf(os.Stdout, "  Reason:     %s\n", last.Reason)
		fmt.Fprintf(os.Stdout, "  Exit Code:  %d\n", last.ExitCode)
		fmt.Fprintf(os.Stdout, "  Finished:   %s\n", last.FinishedAt.Format("15:04:05"))

		// 🔥 УМНЫЕ ПОДСКАЗКИ
		switch last.ExitCode {
		case 137:
			fmt.Fprintf(os.Stdout, "  💥 OOMKilled (не хватает памяти)\n")
		case 1:
			fmt.Fprintf(os.Stdout, "  ❗ Общая ошибка приложения (exit 1)\n")
		case 0:
			fmt.Fprintf(os.Stdout, "  ⚠️ Контейнер завершился нормально, но был перезапущен\n")
		}
	}

	// CURRENT STATE
	if status.State.Waiting != nil {
		fmt.Println("\n⚠️ CURRENT STATE")
		fmt.Printf("  State:           Waiting\n")
		fmt.Printf("  Reason:          %s\n", status.State.Waiting.Reason)
	}

	if status.State.Running != nil {
		fmt.Println("\n⚠️ CURRENT STATE")
		fmt.Printf("  State:           Running\n")
		fmt.Printf("  Started:         %s\n", status.State.Running.StartedAt.Format("15:04:05"))
	}

	// ANALYSIS
	if analysis != "" {
		fmt.Println("\n🧠 ANALYSIS")
		fmt.Printf("  %s\n", analysis)
	}

	// fallback — текущее состояние
	if status.State.Terminated != nil {
		t := status.State.Terminated

		fmt.Fprintf(os.Stdout, "\n💀 CURRENT TERMINATION\n")
		fmt.Fprintf(os.Stdout, "  Reason:     %s\n", t.Reason)
		fmt.Fprintf(os.Stdout, "  Exit Code:  %d\n", t.ExitCode)
	}

	if status.State.Waiting != nil {
		fmt.Fprintf(os.Stdout, "\n⚠️ WAITING\n")
		fmt.Fprintf(os.Stdout, "  Reason: %s\n", status.State.Waiting.Reason)
	}

	// LOGS
	fmt.Println("\n📄 LOGS (previous)")
	printIndented(logs)

	// EVENTS
	fmt.Println("\n📡 EVENTS")
	printIndented(events)

	fmt.Println("\n═══════════════════════════════════════════════")
}

func getPodEvents(c *kubernetes.Clientset, namespace, podName string) string {
	events, err := c.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "❌ Не удалось получить events"
	}

	var result strings.Builder

	for _, e := range events.Items {
		if e.InvolvedObject.Name == podName {
			result.WriteString(fmt.Sprintf(
				"[%s] %s: %s\n",
				e.LastTimestamp.Format("15:04:05"),
				e.Reason,
				e.Message,
			))
		}
	}

	if result.Len() == 0 {
		return "Нет событий"
	}

	return result.String()
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

				// 3. Свежий рестарт, Логи, Масштабирование и Self-Healing
				if status.RestartCount > restartHistory[podKey] {
					totalIncidents++
					fmt.Fprintf(os.Stdout, "\n\033[33m🔥 СВЕЖИЙ ИНЦИДЕНТ #%d! [%s]\033[0m\n", totalIncidents, podKey)

					logs := getPodLogs(c, pod.Namespace, pod.Name)
					if alert := analyzeLogText(logs); alert != "" {
						fmt.Fprintf(os.Stdout, "\033[31m%s\033[0m\n", alert)
					}
					fmt.Fprintf(os.Stdout, "--- LOGS ---\n%s\n------------\n", logs)
					prev := restartHistory[podKey]

					events := getPodEvents(c, pod.Namespace, pod.Name)
					analysis := analyzeLogText(logs)
					printIncidentBlock(
						pod,
						status,
						prev,
						logs,
						events,
						analysis,
					)
					// --- НОВОЕ: Масштабирование (если рестартов 4 и более) ---
					if status.RestartCount > 3 && pod.Namespace == "default" {
						// ❌ Не масштабируем если это проблемы с образом
						if status.State.Waiting != nil {
							reason := status.State.Waiting.Reason
							if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
								fmt.Fprintf(os.Stdout, "🚫 Scaling отключён — проблема с образом\n")
								continue
							}
						}

						fmt.Fprintf(os.Stdout, "🔍 Анализирую необходимость масштабирования для %s...\n", pod.Name)
						scaleUpDeployment(c, pod.Namespace, pod.Name)
					}

					// --- SELF-HEALING (если рестартов более 10) ---
					if status.RestartCount > 10 && pod.Namespace != "kube-system" {
						fmt.Fprintf(os.Stdout, "🧹 \033[35mSELF-HEALING: Удаляю %s...\033[0m\n", podKey)
						c.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
						delete(restartHistory, podKey)
						continue // Переходим к следующему поду
					}

					// ВАЖНО: Обновляем историю в самом конце этого блока
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
	limit := int64(20)

	opts := &corev1.PodLogOptions{
		TailLines: &limit,
		Previous:  true, // 🔥 ВАЖНО — логи до падения
	}

	req := c.CoreV1().Pods(namespace).GetLogs(name, opts)
	podLogs, err := req.DoRaw(context.TODO())
	if err != nil {
		return "❌ Previous логи недоступны"
	}
	return string(podLogs)
}

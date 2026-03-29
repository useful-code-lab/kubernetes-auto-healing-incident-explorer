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
    fmt.Println("--- ПРИЛОЖЕНИЕ ЗАПУСКАЕТСЯ ---")
    config, err := rest.InClusterConfig()
    if err != nil {
        fmt.Printf("КРИТИЧЕСКАЯ ОШИБКА КОНФИГА: %v\n", err)
        return
    }

    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        fmt.Printf("КРИТИЧЕСКАЯ ОШИБКА КЛИЕНТА: %v\n", err)
        return
    }

    mode := "status"
    if len(os.Args) > 1 {
        mode = os.Args[1] // Убедитесь, что индекс [1], так как [0] - имя программы
    }
    fmt.Printf("ВЫБРАН РЕЖИМ: %s\n", mode)

    if mode == "events" {
        monitorEvents(clientset)
    } else {
        showStatus(clientset)
    }
}

func monitorEvents(c *kubernetes.Clientset) {
    fmt.Println(">>> Инициализация Watcher для Events...")
    watcher, err := c.CoreV1().Events("").Watch(context.TODO(), metav1.ListOptions{})
    if err != nil {
        fmt.Printf("ОШИБКА WATCH: %v\n", err)
        return
    }
    fmt.Println(">>> Канал Watch успешно открыт. Ожидание событий...")

    for event := range watcher.ResultChan() {
        // Добавим проверку типа события
        fmt.Printf("--- ПОЛУЧЕНО СОБЫТИЕ ТИПА: %v ---\n", event.Type)
        
        e, ok := event.Object.(*corev1.Event)
        if !ok {
            fmt.Println("Ошибка приведения типа события")
            continue
        }
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

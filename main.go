package main

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	// Создаем конфиг для работы ВНУТРИ кластера
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Printf("Ошибка конфига: %v\n", err)
		os.Exit(1)
	}

	// Создаем клиент (здесь сработает наша правка в staging)
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	// Просто выводим список подов в default
	pods, _ := clientset.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	fmt.Printf("В неймспейсе default найдено %d подов\n", len(pods.Items))
}

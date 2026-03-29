## Kubernetes dashboard

http://127.0.0.1:41099/api/v1/namespaces/kubernetes-dashboard/services/http:kubernetes-dashboard:/proxy/#/cronjob?namespace=default

![Скриншот терминала](/images/kubernetes-dashboard.png)

## Подключение к kubernetes-staging
Нужно рядом с директорией проекта склонировать  проект ``git clone git@github.com:avpavlov-cloud/kube-source-explorer.git``

Сборка докера
```bash
eval $(minikube docker-env)

aleksey@aleksey:~/kube-source-explorer$ cd ..
aleksey@aleksey:~$ docker build -t kube-source-explorer:latest -f kube-source-explorer/Dockerfile .
```

## Применяем права и создаем под
```bash
kubectl apply -f kube-source-explorer/manifests/rbac.yaml

kubectl apply -f kube-source-explorer/manifests/pod.yaml
```

Проверить наличие образа внутри Minikube
```bash
minikube image ls --format table | grep kube-source-explorer
```

## Запустить под
```bash
minikube start --driver=docker
```
```bash
# Проверить статус
kubectl get pod kube-source-explorer

# Посмотреть вывод программы и вашу "пасхалку" из staging
kubectl logs kube-source-explorer
```

В проекте работают команды make

Меняем настройки в manifests и применяем `kubectl replace --force -f manifests/pod.yaml`

Что мы в итоге реализовали:
Интеграция со Staging: Проект успешно использует «живой» код из локального репозитория Kubernetes (client-go, api, apimachinery).
Сложная сборка: Dockerfile умеет подтягивать зависимости из родительской папки, кэшировать слои и собирать статический бинарник под Go 1.26.
Гибкое управление: Через args в манифесте вы переключаете логику работы (мониторинг событий или аудит подов) без пересборки образа.
Безопасность (RBAC): Создана кастомная роль для работы с системными событиями кластера.
Автоматизация: Makefile позволяет одной командой make redo обновить код в кластере.




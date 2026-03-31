# 🚀 kube-source-explorer

Инструмент для **наблюдения, диагностики и автоматической реакции на проблемы в Kubernetes-кластере**.

Проект работает внутри кластера (InClusterConfig) и выполняет:

* 📡 мониторинг событий Kubernetes
* 🔥 детекцию инцидентов (рестарты контейнеров)
* 🧠 анализ логов
* 📈 экспорт метрик в Prometheus
* ⚡ авто-масштабирование (scale up)
* 🧹 self-healing (удаление нестабильных pod'ов)

---

# 📊 Kubernetes Dashboard

Доступ к UI:

```
http://127.0.0.1:41099/api/v1/namespaces/kubernetes-dashboard/services/http:kubernetes-dashboard:/proxy/#/cronjob?namespace=default
```

![Скриншот терминала](/images/kubernetes-dashboard.png)

![Скриншот терминала](/images/kubernetes-dashboard2.png)

---

# ⚙️ Подключение к staging

Клонируем проект рядом:

```bash
git clone git@github.com:avpavlov-cloud/kube-source-explorer.git
```

---

# 🐳 Сборка Docker-образа

Minikube использует свой Docker daemon, поэтому:

```bash
eval $(minikube docker-env)
```

Сборка:

```bash
docker build -t kube-source-explorer:latest -f kube-source-explorer/Dockerfile .
```

---

# 🔐 RBAC и запуск Pod

Применяем права:

```bash
kubectl apply -f kube-source-explorer/manifests/rbac.yaml
```

Создаем pod:

```bash
kubectl apply -f kube-source-explorer/manifests/pod.yaml
```

---

# ✅ Проверка образа в Minikube

```bash
minikube image ls --format table | grep kube-source-explorer
```

---

# ▶️ Запуск

```bash
minikube start --driver=docker
```

Проверка:

```bash
kubectl get pod kube-source-explorer
kubectl logs kube-source-explorer
```

---

# 🧠 Архитектура приложения

Приложение состоит из нескольких логических модулей:

### 1. Мониторинг состояния (основной цикл)

* опрашивает все pod'ы каждые 5 секунд
* отслеживает:

  * рестарты контейнеров
  * зависшие состояния (ImagePullBackOff и др.)
  * изменения статуса

---

### 2. Детекция инцидентов

Инцидент фиксируется, если:

```text
RestartCount увеличился
```

При этом система:

* увеличивает счетчик `totalIncidents`
* собирает:

  * логи (previous)
  * события Kubernetes
* запускает анализ

---

### 3. Анализ логов

Функция `analyzeLogText` ищет:

| Ошибка               | Интерпретация       |
| -------------------- | ------------------- |
| `out of memory`      | 💥 OOMKilled        |
| `panic`              | 🔥 crash приложения |
| `connection refused` | 🌐 проблемы сети    |
| `permission denied`  | 🔐 проблемы прав    |

---

### 4. Формирование отчета

Каждый инцидент выводится как блок:

```
🔥 INCIDENT
Pod: default/app-123
Restarts: 1 → 2

💀 LAST TERMINATION
Exit Code: 137

📄 LOGS
📡 EVENTS
🧠 ANALYSIS
```

---

# ⚡ Автоматическое масштабирование

Если:

```text
RestartCount > 3
```

система пытается увеличить Deployment:

### Защиты от "плохого scaling":

* ⛔ лимит реплик (`maxReplicas = 10`)
* ⏳ cooldown (`30 секунд`)
* ⚠️ проверка готовности pod'ов
* 🚫 отключение при ImagePull ошибках

---

# 🧹 Self-healing

Если pod совсем нестабилен:

```text
RestartCount > 10
```

➡️ pod удаляется:

```go
kubectl delete pod
```

Kubernetes сам пересоздаст его.

---

# 📡 Режимы работы

Режим задается аргументом:

```bash
/app [mode]
```

## 1. status (по умолчанию)

* мониторинг pod'ов
* анализ рестартов
* autoscaling
* self-healing

## 2. events

```bash
/app events
```

* watch всех событий Kubernetes
* вывод в реальном времени

---

# 🧾 Метрики (Prometheus)

Сервис поднимается на:

```
:8080/metrics
```

Доступ:

```bash
kubectl port-forward kube-source-explorer 8080:8080
```

Открыть:

```
http://localhost:8080/metrics
```

---

## Метрики:

### 1. Общее количество инцидентов

```
explorer_incidents_total
```

### 2. Аптайм приложения

```
explorer_uptime_seconds
```

---

# 🔄 Управление через manifests

Изменение режима:

```yaml
command: ["/app", "events"]
```

Применение:

```bash
kubectl replace --force -f manifests/pod.yaml
```

---

# 🧪 Тестирование

## 1. Краш контейнера

```bash
kubectl run immediate-fail --image=busybox -- /bin/sh -c "echo 'ERROR'; exit 1"
```

Ожидаем:

* 🔥 INCIDENT
* анализ логов
* увеличение счетчика

---

## 2. Масштабирование

```bash
kubectl create deployment scaling-demo --image=busybox -- /bin/sh -c "exit 1"
```

Проверка:

```bash
kubectl get pods -l app=scaling-demo
```

---

## 3. Проверка метрик

```bash
make redo
kubectl port-forward kube-source-explorer 8080:8080
```

---

# ⚙️ Makefile команды

```bash
make redo
```

Что делает:

* пересборка образа
* обновление в кластере
* перезапуск pod

---

# 🔐 Безопасность (RBAC)

Приложение использует кастомную роль:

* доступ к pod'ам
* доступ к events
* доступ к deployment (для scaling)

---

# 📦 Особенности сборки

Dockerfile:

* использует локальные зависимости Kubernetes:

  * client-go
  * api
  * apimachinery
* собирает статический бинарник
* оптимизирует кэш слоев

---

# 🧩 Что реализовано

✔ Интеграция с Kubernetes API
✔ Реальное наблюдение за кластером
✔ Анализ логов
✔ Автоматическое масштабирование
✔ Self-healing
✔ Экспорт метрик
✔ Работа без внешних сервисов

---

# 🚧 Ограничения

* нет persistence (всё в памяти)
* простая логика анализа (string matching)
* нет rate-limiting на события
* работает только внутри кластера

---

# 💡 Возможные улучшения

* интеграция с Prometheus Alertmanager
* webhook/Slack уведомления
* ML-анализ логов
* хранение истории (Redis / PostgreSQL)
* поддержка HPA вместо ручного scaling

---

# 🏁 Итог

Проект демонстрирует полноценный цикл:

```
Detect → Analyze → React → Heal → Measure
```

И фактически является мини-версией:

* Prometheus + Alertmanager
* Kubernetes Operator
* Incident Response системы



## Kubernetes dashboard

http://127.0.0.1:41099/api/v1/namespaces/kubernetes-dashboard/services/http:kubernetes-dashboard:/proxy/#/cronjob?namespace=default

![Скриншот терминала](/images/kubernetes-dashboard.png)

## Подключение к kubernetes-staging
Нужно рядом с директорией проекта склонировать  проект ``git clone git@github.com:avpavlov-cloud/kube-source-explorer.git``

Сборка докера
```bash
aleksey@aleksey:~/kube-source-explorer$ cd ..
aleksey@aleksey:~$ docker build -t kube-source-explorer:latest -f kube-source-explorer/Dockerfile .
```

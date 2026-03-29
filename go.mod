module github.com/avpavlov-cloud/kube-source-explorer

go 1.26.1

// Указываем основной зависимости версию (она подменится локальной)
require (
    k8s.io/client-go v0.0.0
    k8s.io/api v0.0.0
    k8s.io/apimachinery v0.0.0
)

// Магия: перенаправляем поиск пакетов в соседнюю папку kubernetes/staging
replace (
    k8s.io/client-go => ../kubernetes/staging/src/k8s.io/client-go
    k8s.io/api => ../kubernetes/staging/src/k8s.io/api
    k8s.io/apimachinery => ../kubernetes/staging/src/k8s.io/apimachinery
)

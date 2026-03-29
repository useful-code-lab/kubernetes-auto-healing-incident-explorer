IMAGE_NAME = kube-source-explorer
TAG = latest

# 1. Сборка образа
build:
	@echo "--- Сборка из родительского контекста ---"
	@eval $$(minikube docker-env) && \
	docker build -t $(IMAGE_NAME):$(TAG) -f Dockerfile ..
	# Мы передаем '..' (родительскую папку) как контекст, 
	# чтобы Docker видел и kubernetes, и наш проект.

# 2. Перезапуск пода
redo: build
	kubectl delete pod $(IMAGE_NAME) --ignore-not-found
	kubectl apply -f manifests/pod.yaml

# 3. Логи
logs:
	kubectl logs -f $(IMAGE_NAME)

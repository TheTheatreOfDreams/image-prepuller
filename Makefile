KIND_CLUSTER_NAME ?= image-prepuller
IMG ?= ghcr.io/thetheatreofdreams/image-prepuller:latest

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make kind-up       Create kind cluster, deploy operator, and deploy nginx"
	@echo "  make kind-create   Create the kind cluster if it does not exist"
	@echo "  make docker-build  Build the operator image"
	@echo "  make kind-load     Load the operator image into the kind cluster"
	@echo "  make deploy        Deploy the operator manifests"
	@echo "  make deploy-nginx  Deploy the sample nginx workload"
	@echo "  make images        List Image custom resources discovered by the operator"
	@echo "  make kind-delete   Delete the kind cluster"

.PHONY: kind-up
kind-up: kind-create docker-build kind-load deploy deploy-nginx images

.PHONY: kind-create
kind-create:
	@kind get clusters | grep -qx "$(KIND_CLUSTER_NAME)" || kind create cluster --name "$(KIND_CLUSTER_NAME)"
	kubectl cluster-info --context "kind-$(KIND_CLUSTER_NAME)"

.PHONY: docker-build
docker-build:
	docker build -t "$(IMG)" .

.PHONY: kind-load
kind-load:
	kind load docker-image "$(IMG)" --name "$(KIND_CLUSTER_NAME)"

.PHONY: deploy
deploy:
	kubectl apply -k config/default
	kubectl -n image-prepuller-system rollout status deployment/image-prepuller-controller-manager --timeout=120s

.PHONY: deploy-nginx
deploy-nginx:
	kubectl apply -f config/samples/nginx.yaml
	kubectl rollout status deployment/nginx --timeout=120s

.PHONY: images
images:
	kubectl get images.image-prepuller.theatreofdreams.io

.PHONY: kind-delete
kind-delete:
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"

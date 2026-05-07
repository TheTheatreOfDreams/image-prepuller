KIND_CLUSTER_NAME ?= prepuller
KUBE_CONTEXT ?= kind-$(KIND_CLUSTER_NAME)
KUBECTL ?= kubectl --context "$(KUBE_CONTEXT)"
IMG ?= ghcr.io/thetheatreofdreams/prepuller:v0.0.1
GHCR_SECRET_NAME ?= ghcr

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make kind-up       Create kind cluster, deploy operator from GHCR, and deploy nginx"
	@echo "  make kind-up-private Create kind cluster, deploy private GHCR operator, and deploy nginx"
	@echo "  make kind-create   Create the kind cluster if it does not exist"
	@echo "  make docker-build  Build the operator image"
	@echo "  make kind-load     Load the operator image into the kind cluster"
	@echo "  make deploy        Deploy the operator manifests"
	@echo "  make deploy-private Deploy the operator using GHCR image pull credentials"
	@echo "  make ghcr-secret   Create/update the GHCR image pull secret"
	@echo "  make auth-check    Check controller RBAC in the kind cluster"
	@echo "  make deploy-nginx  Deploy the sample nginx workload"
	@echo "  make images        List Image custom resources discovered by the operator"
	@echo "  make kind-delete   Delete the kind cluster"

.PHONY: kind-up
kind-up: kind-create deploy deploy-nginx images

.PHONY: kind-up-private
kind-up-private: kind-create deploy-private deploy-nginx images

.PHONY: kind-create
kind-create:
	@kind get clusters | grep -qx "$(KIND_CLUSTER_NAME)" || kind create cluster --name "$(KIND_CLUSTER_NAME)"
	$(KUBECTL) cluster-info

.PHONY: docker-build
docker-build:
	docker build -t "$(IMG)" .

.PHONY: kind-load
kind-load:
	kind load docker-image "$(IMG)" --name "$(KIND_CLUSTER_NAME)"

.PHONY: deploy
deploy:
	$(KUBECTL) apply -k config/default
	$(KUBECTL) -n prepuller-system rollout status deployment/prepuller-controller-manager --timeout=120s

.PHONY: deploy-private
deploy-private:
	$(KUBECTL) apply -k config/default
	$(MAKE) ghcr-secret
	$(KUBECTL) -n prepuller-system patch serviceaccount prepuller-controller-manager --type merge -p '{"imagePullSecrets":[{"name":"$(GHCR_SECRET_NAME)"}]}'
	$(KUBECTL) -n prepuller-system rollout restart deployment/prepuller-controller-manager
	$(KUBECTL) -n prepuller-system rollout status deployment/prepuller-controller-manager --timeout=120s

.PHONY: ghcr-secret
ghcr-secret:
ifndef GHCR_USERNAME
	$(error GHCR_USERNAME is required, for example GHCR_USERNAME=your-github-user)
endif
ifndef GHCR_TOKEN
	$(error GHCR_TOKEN is required. Use a GitHub token with read:packages)
endif
	$(KUBECTL) create namespace prepuller-system --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(KUBECTL) -n prepuller-system create secret docker-registry "$(GHCR_SECRET_NAME)" \
		--docker-server=ghcr.io \
		--docker-username="$(GHCR_USERNAME)" \
		--docker-password="$(GHCR_TOKEN)" \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -

.PHONY: auth-check
auth-check:
	$(KUBECTL) auth can-i list pods --as=system:serviceaccount:prepuller-system:prepuller-controller-manager
	$(KUBECTL) auth can-i watch pods --as=system:serviceaccount:prepuller-system:prepuller-controller-manager
	$(KUBECTL) auth can-i list images.prepuller.theatreofdreams.io --as=system:serviceaccount:prepuller-system:prepuller-controller-manager
	$(KUBECTL) auth can-i watch images.prepuller.theatreofdreams.io --as=system:serviceaccount:prepuller-system:prepuller-controller-manager

.PHONY: deploy-nginx
deploy-nginx:
	$(KUBECTL) apply -f config/samples/nginx.yaml
	$(KUBECTL) rollout status deployment/nginx --timeout=120s

.PHONY: images
images:
	$(KUBECTL) get images.prepuller.theatreofdreams.io

.PHONY: kind-delete
kind-delete:
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"

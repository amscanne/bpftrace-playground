PROJECT ?= bpftrace-playground
REGION ?= us-central1
SERVICE_ACCOUNT_NAME ?= bpftrace-playground
SERVICE_ACCOUNT_EMAIL ?= $(SERVICE_ACCOUNT_NAME)@$(PROJECT).iam.gserviceaccount.com
IMAGE_NAME ?= bpftrace-playground
IMAGE_TAG ?= latest
REPO ?= bpftrace-playground
IMAGE_URI = $(REGION)-docker.pkg.dev/$(PROJECT)/$(REPO)/$(IMAGE_NAME):$(IMAGE_TAG)

GO_FILES := $(shell find . -type f -name '*.go')
STATIC_FILES := $(shell find templates -type f)
OTHER_FILES := flake.nix go.mod go.sum
NIX_SHELL = nix develop -c --

.PHONY: all push repo service-account deploy clean

all: deploy

# Build the container using Nix if source files have changed.
image: $(GO_FILES) $(STATIC_FILES) $(OTHER_FILES)
	@echo "--> Building container with Nix..."
	@nix build .#default -o $@

# Push the container to Google Artifact Registry using skopeo.
push: image repo
	@echo "--> Pushing image to $(IMAGE_URI) with skopeo..."
	@$(NIX_SHELL) gcloud auth print-access-token | $(NIX_SHELL) skopeo copy --dest-creds "oauth2accesstoken:$$(cat)" docker-archive:image docker://$(IMAGE_URI)

# Ensure the Artifact Registry repository exists.
repo:
	@echo "--> Checking for Artifact Registry repository $(REPO) in region $(REGION)..."
	@$(NIX_SHELL) gcloud artifacts repositories describe $(REPO) --location=$(REGION) --project=$(PROJECT) >/dev/null 2>&1 || \
		(echo "--> Repository not found, creating..." && \
		$(NIX_SHELL) gcloud artifacts repositories create $(REPO) \
			--repository-format=docker \
			--location=$(REGION) \
			--description="Repository for bpftrace-playground images" \
			--project=$(PROJECT))

# Ensure the service account exists.
service-account:
	@echo "--> Checking for service account $(SERVICE_ACCOUNT_EMAIL)..."
	@$(NIX_SHELL) gcloud iam service-accounts describe $(SERVICE_ACCOUNT_EMAIL) --project=$(PROJECT) >/dev/null 2>&1 || \
		(echo "--> Service account not found, creating..." && \
		$(NIX_SHELL) gcloud iam service-accounts create $(SERVICE_ACCOUNT_NAME) \
			--display-name="bpftrace-playground runner" \
			--description="Service account for bpftrace-playground" \
			--project=$(PROJECT))

# Deploy the service to Cloud Run.
deploy: push service-account
	@echo "--> Deploying service to Cloud Run in region $(REGION)..."
	@$(NIX_SHELL) gcloud run deploy $(IMAGE_NAME) \
		--image=$(IMAGE_URI) \
		--service-account=$(SERVICE_ACCOUNT_EMAIL) \
		--region=$(REGION) \
		--project=$(PROJECT) \
		--port=8080 \
		--concurrency=1 \
		--max-instances=10 \
		--timeout=300 \
		--memory=1Gi \
		--cpu=1 \
		--execution-environment=gen2 \
		--allow-unauthenticated

clean:
	@echo "Cleaning up..."
	@go clean -modcache
	@rm -rf .go-build vendor
	@rm -f image

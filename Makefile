BINARY      = machinecfg
MODULE      = github.com/mgrzybek/machinecfg
GO_VERSION  = 1.24
LAST_COMMIT = $(shell git rev-parse HEAD)
TMP         = tmp
TEST        = test

##############################################################################
# Go: usual targets

${BINARY}: go.mod ## Build the program (default target)
	CGO_ENABLED=0 go build -o ${BINARY} main.go

.PHONY: all
all: ${BINARY} ## Create the program

.PHONY: dist-clean
dist-clean: ## Clean the created artifacts
	rm -f ${BINARY} c.out

.PHONY: clean
clean: ## Clean the temporary files
	cd $(TMP) && docker compose down
	rm -rf $(TMP)/*

	for d in $(docker ps --all | awk '!/ID/ {print $$1}') ; do docker rm $$d ; done
	for v in $(docker volume ls | awk '!/DRIVER/{print $$2}') ; do docker volume rm $$v ; done

.PHONY: fmt
fmt: ## Format the sources
	find . -type f -name "*.go" -exec go fmt {} \;

.PHONY: docs
docs: ## Generate the SVG drawings
	$(MAKE) -C docs all

##############################################################################
# Testing: offline and online

test/netbox.sql: ## Create the netbobx dump
	cd $(TMP)/netbox && docker compose exec -u postgres postgres bash -c "pg_dump -U netbox" > ../../$(TEST)/netbox.sql

.PHONY: netbox-prepare
netbox-prepare: ## Deploy netbox using docker compose and restore dump
	@echo "Cloning netbox-docker and copying the dump to load"
	test -d $(TMP)/netbox || git clone -b release https://github.com/netbox-community/netbox-docker.git $(TMP)/netbox
	mkdir -p $(TMP)/netbox/docker-entrypoint-initdb.d
	cp $(TEST)/docker-compose.override.yml $(TMP)/netbox
	cp $(TEST)/netbox.sql $(TMP)/netbox/docker-entrypoint-initdb.d

	@echo "Starting the stack"
	cd $(TMP)/netbox && docker compose pull
	cd $(TMP)/netbox && docker compose up -d

	@echo "Creating the API token"
	awk '/Token for machinecfg/ {print $$5}' $(TEST)/netbox.sql > $(TMP)/token

.PHONY: test
test: fmt ${BINARY} ## Run go test
	go test -v -race -buildvcs ./...

.PHONY: test-online
test-online: ${BINARY} netbox-prepare ## Run tests against a remote Netbox endpoint
	@echo TODO

.PHONY: validate
validate: ${BINARY} ## Start a validation process of Netbox objects
	@echo TODO

##############################################################################
# Go: packaging targets

.PHONY: get
get: ## Download required modules
	go get ./...

go.mod:
	go mod init ${MODULE}
	go mod tidy

##############################################################################
# Go: quality targets

c.out: ## Create the coverage file
	go test ./... -coverprofile=c.out

.PHONY: coverage
coverage: c.out ## Show the coverage ratio per function
	go tool cover -func=c.out

.PHONY: coverage-code
coverage-code: c.out ## Show the covered code in a browser
	go tool cover -html=c.out

.PHONY: audit ## Run quality control checks
audit:
	go mod verify
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	go test -race -buildvcs -vet=off ./...

##############################################################################
# Help

.PHONY: help
help: ## This help message
	@awk -F: \
		'/^([a-z\.-]+): *.* ## (.+)$$/ {gsub(/: .*?\s*##/, "\t");print}' \
		Makefile \
	| expand -t20 \
	| sort

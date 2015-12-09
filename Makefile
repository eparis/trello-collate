default: trello-collate

all: push

DATE := $(shell date +%F)
GIT := $(shell git rev-parse --short HEAD)

TAG ?= $(DATE)-$(GIT)

ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
AUTH ?= "$(ROOT_DIR)/auth.yaml"

trello-collate:
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o trello-collate

update_pod_version:
	sed -i -e 's|[[:digit:]]\{4\}-[[:digit:]]\{2\}-[[:digit:]]\{2\}-[[:xdigit:]]\+|$(TAG)|g' rc.yaml

auth2secret:
	./auth2secret.sh

container: trello-collate update_pod_version
	docker build -t docker.io/eparis/trello-collate:$(TAG) .

local_run: container
	docker run --rm -v $(AUTH):/auth.yaml docker.io/eparis/trello-collate:$(TAG) /trello-collate --once

push: container
	docker push docker.io/eparis/trello-collate:$(TAG)

clean:
	rm -f trello-collate

.PHONY: all trello-collate update_pod_version container push clean local_run auth2secret default

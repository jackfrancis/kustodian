.DEFAULT: all
.PHONY: all clean image publish-image minikube-publish

DH_ORG=jackfrancis
VERSION=$(shell git symbolic-ref --short HEAD)-$(shell git rev-parse --short HEAD)
SUDO=$(shell docker info >/dev/null 2>&1 || echo "sudo -E")

all: image

clean:
	rm -f cmd/kustodian/kustodian
	rm -rf ./build

godeps=$(shell go list -f '{{join .Deps "\n"}}' $1 | grep -v /vendor/ | xargs go list -f '{{if not .Standard}}{{ $$dep := . }}{{range .GoFiles}}{{$$dep.Dir}}/{{.}} {{end}}{{end}}')

DEPS=$(call godeps,./cmd/kustodian)

cmd/kustodian/kustodian: $(DEPS)
cmd/kustodian/kustodian: cmd/kustodian/*.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $@ cmd/kustodian/*.go

build/.image.done: cmd/kustodian/Dockerfile cmd/kustodian/kustodian
	mkdir -p build
	cp $^ build
	$(SUDO) docker build -t docker.io/$(DH_ORG)/kustodian -f build/Dockerfile ./build
	$(SUDO) docker tag docker.io/$(DH_ORG)/kustodian docker.io/$(DH_ORG)/kustodian:$(VERSION)
	touch $@

image: build/.image.done

publish-image: image
	$(SUDO) docker push docker.io/$(DH_ORG)/kustodian:$(VERSION)

minikube-publish: image
	$(SUDO) docker save docker.io/$(DH_ORG)/kustodian | (eval $$(minikube docker-env) && docker load)

GOOS:=linux
VERSION:=$(shell git describe --abbrev=4 --always --tags)
CONTAINER=mesos-dns

version:
	@echo "##teamcity[setParameter name='env.DOCKER_TAG' value='$(VERSION)']"

clean:
	docker rmi $(CONTAINER)-build || true
	docker rmi $(CONTAINER) || true

dockerbuild:
	docker build --no-cache -t $(CONTAINER)-build -f Dockerfile.test .

docker: buildstatic
	docker build --no-cache -t $(CONTAINER):$(VERSION) -f Dockerfile.build .

# Ignore vendored dependencies
# Return 1 if any files found that haven't been formatted
fmttest: dockerbuild
	docker run --rm $(CONTAINER)-build gofmt -l . | awk '!/^Godep/ { print $0; err=1 }; END{ exit err }'

test: dockerbuild
	docker run --rm $(CONTAINER)-build godep go test -v ./...
	docker run --rm $(CONTAINER)-build godep go test -v -short -race ./...

testall: fmttest test 

build: dockerbuild
	docker run --rm -v "$(PWD)/build":/build -e GOOS=$(GOOS) $(CONTAINER)-build godep go build -ldflags="-X main.Version=$(VERSION)" -o /build/mesos-dns
	
buildstatic: dockerbuild
	docker run --rm -v "$(PWD)/build":/build -e CGO_ENABLED=0 -e GOOS=$(GOOS) $(CONTAINER)-build godep go build -a -installsuffix cgo -ldflags "-s -X main.Version=$(VERSION)" -o /build/mesos-dns

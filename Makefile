OS = darwin freebsd linux openbsd
ARCHS = 386 amd64 arm

all: build release

build: deps
	go build

release: clean deps
	@for arch in $(ARCHS);\
	do \
		for os in $(OS);\
		do \
			echo "Building $$os-$$arch"; \
			mkdir -p build/tachyon-$$os-$$arch/; \
			GOOS=$$os GOARCH=$$arch go build -o build/tachyon-$$os-$$arch/tachyon; \
			tar cz -C build -f build/tachyon-$$os-$$arch.tar.gz tachyon-$$os-$$arch; \
		done \
	done

test: deps
	go test ./...

deps:
	go get -d -v -t ./...

clean:
	rm -rf build
	rm -f tachyon

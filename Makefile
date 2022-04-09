PKGNAME=$(shell go list)

build:
	@go build ${PKGNAME}/tools/recoverlinter

.PHONY: clean
clean:
	@rm recoverlinter

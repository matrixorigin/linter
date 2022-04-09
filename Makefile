PKGNAME=$(shell go list)

build:
	@go build ${PKGNAME}/cmd/molint

.PHONY: clean
clean:
	@rm molint

# MatrixOrigin Linter

This repo contains custom linters developed by MatrixOrigin.

## Installation

```
go install github.com/matrixorigin/linter/cmd/molint@latest
```

## Usage

```
cd matrixone
go vet -vettool=$(which molint) ./...
```

## Linters

### checkrecover

As required by our [error handling rules](https://github.com/matrixorigin/matrixone/blob/main/pkg/common/moerr/error_handling.md),
"You should not recover any panics unless it is at a few well defined places".
This linter helps to enforce that. 

It will fail when checking the frontend folder as recover() calls there are not currently marked as approved. 

### pkgblocklist

Block unwanted packages from importing.


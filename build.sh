#!/bin/sh

# Disable modules.  TODO: try again after guru supports modules:
# https://golang.org/issue/24661
#
# # Opt into Go modules.
# # https://github.com/golang/go/wiki/Modules#how-to-install-and-activate-module-support
# GO111MODULE=on
#
# Opt out of Go modules.
GO111MODULE=off

export GO111MODULE
go test ./... && go build ./...

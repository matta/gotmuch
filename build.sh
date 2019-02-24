#!/bin/sh

# This codifies an incantation described at
# https://github.com/golang/go/wiki/Modules#how-to-install-and-activate-module-support
GO111MODULE=on
export GO111MODULE

go build ./...

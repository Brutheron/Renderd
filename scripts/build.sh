#!/bin/sh
set -eu

mkdir -p bin
go build -trimpath -o bin/renderd ./cmd/renderd

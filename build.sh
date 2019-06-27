#! /bin/bash

# build project
rm -rf ./artifacts
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o artifacts/metrics-client

# build docker container

docker build -t morfien101/metrics-client:latest .
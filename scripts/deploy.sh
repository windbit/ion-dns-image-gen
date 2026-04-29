#!/usr/bin/env bash

set -e

docker build --platform linux/amd64 -t ghcr.io/windbit/ion-dns-image-gen .
docker push ghcr.io/windbit/ion-dns-image-gen

ssh infra 'set -e
cd /srv/ion-dns
docker compose pull dns-image-gen
docker compose up -d'

#!/usr/bin/env nix-shell
#!nix-shell -i bash ./shell.nix

set -eux

PROJECT_NAME=bonk-api

nix build .#packages.x86_64-linux.dockerImage
skopeo \
    --insecure-policy \
    copy docker-archive:"$(realpath ./result)" \
    docker://registry.fly.io/$PROJECT_NAME:latest \
    --dest-creds x:"$(flyctl auth token)" \
    --format v2s2

flyctl deploy -i registry.fly.io/$PROJECT_NAME:latest --remote-only

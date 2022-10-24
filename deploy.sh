#!/usr/bin/env nix-shell
#!nix-shell -i bash ./shell.nix

set -euxo pipefail

PROJECT_NAME=bonk-api

nix build .#packages.x86_64-linux.dockerImage
# note: will write auth token to XDG_RUNTIME_DIR
flyctl auth token | skopeo login -u x --password-stdin registry.fly.io
skopeo \
    --insecure-policy \
    copy docker-archive:"$(realpath ./result)" \
    docker://registry.fly.io/$PROJECT_NAME:latest \
    --format v2s2

flyctl deploy -i registry.fly.io/$PROJECT_NAME:latest --remote-only

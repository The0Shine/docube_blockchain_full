#!/usr/bin/env bash
# Entrypoint for blockchain-service mode
# Copies default config if not overridden, then starts the Go service

set -e

# Copy default service config if no override mounted
if [ ! -f "/app/config/app.yaml" ]; then
    mkdir -p /app/config
    cp -r /fabric/service-config/* /app/config/
fi

cd /app
exec blockchain-service

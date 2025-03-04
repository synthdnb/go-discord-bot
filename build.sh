#!/bin/bash

# Exit on error
set -e

echo "Building Docker image for go-discord-bot..."
echo "Platform: linux/amd64"
echo "Tag: synthdnb/go-discord-bot"

# Build the Docker image for linux/amd64 platform
docker buildx build --platform linux/amd64 -t synthdnb/go-discord-bot:latest .

echo "Build completed successfully!"
echo "To push the image to Docker Hub, run:"
echo "docker push synthdnb/go-discord-bot:latest"

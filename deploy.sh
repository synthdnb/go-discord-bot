#!/bin/bash

# Exit on error
set -e

# Configuration - Replace these with your actual values
REMOTE_SERVER="pikachu.io"
REMOTE_USER="ec2-user"
CONTAINER_NAME="discord-bot"
ENV_FILE="/home/ec2-user/go-discord-bot/env"

echo "Pushing Docker image to Docker Hub..."
docker push synthdnb/go-discord-bot:latest

echo "Deploying to remote server $REMOTE_SERVER..."
ssh $REMOTE_USER@$REMOTE_SERVER << ENDSSH
  # Pull the latest image
  docker pull synthdnb/go-discord-bot:latest

  # Stop the existing container if it's running
  if [ "\$(docker ps -q -f name=${CONTAINER_NAME})" ]; then
    echo "Stopping existing container..."
    docker stop ${CONTAINER_NAME}
    docker rm ${CONTAINER_NAME}
  fi

  # Start a new container
  echo "Starting new container..."
  docker run -d \\
    --name ${CONTAINER_NAME} \\
    --restart unless-stopped \\
    --env-file ${ENV_FILE} \\
    synthdnb/go-discord-bot:latest

  echo "Deployment completed!"
ENDSSH

echo "Deployment script finished!"

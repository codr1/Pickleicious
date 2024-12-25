#!/bin/bash

# scripts/dev_watch.sh

# Function to cleanup background processes on exit
cleanup() {
    echo "Stopping all processes..."
    pkill -P $$
}

# Set up cleanup on script exit
trap cleanup EXIT

# Start Templ watcher
echo "Starting Templ watcher..."
templ generate --watch &

# Start Tailwind watcher
echo "Starting Tailwind watcher..."
tailwindcss -i ./web/styles/main.css -o ./build/bin/static/css/main.css --watch &

# Start Go development server with hot reload
echo "Starting Go server..."
go run ./cmd/server/main.go &

# Wait for all background processes
wait

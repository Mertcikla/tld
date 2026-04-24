#!/bin/sh

# Get the root directory of the repository
REPO_ROOT=$(git rev-parse --show-toplevel)
cd "$REPO_ROOT"

echo "Running pre-commit hooks..."

# Run Backend Lint
echo "Linting Backend (Go)..."
make lint-be
if [ $? -ne 0 ]; then
    echo "Backend linting failed. Please fix the issues and try again."
    exit 1
fi

# Run Frontend Lint
echo "Linting Frontend (TypeScript)..."
if [ -d "frontend" ] && [ -f "frontend/package.json" ]; then
    make lint-fe
    if [ $? -ne 0 ]; then
        echo "Frontend linting failed. Please fix the issues and try again."
        exit 1
    fi
fi

echo "All checks passed!"
exit 0

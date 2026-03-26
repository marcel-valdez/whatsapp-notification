#!/usr/bin/env bash
# --- Configuration ---
BINARY_NAME="whatsapp-notification"
SOURCE_FILE="main.go"
BUILD_DIR="./bin"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting build process for $BINARY_NAME...${NC}"

# 1. Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed. Please install Go to continue.${NC}"
    exit 1
fi

# 2. Check for GCC (Required for SQLite/CGO)
if ! command -v gcc &> /dev/null; then
    echo -e "${RED}Error: GCC not found. SQLite requires a C compiler.${NC}"
    exit 1
fi

# 3. Create build directory if it doesn't exist
mkdir -p $BUILD_DIR

# 4. Tidy modules and download dependencies
echo "Tidying Go modules..."
go mod tidy

# 5. Compile the binary
echo "Compiling..."
# CGO_ENABLED=1 is required for the go-sqlite3 driver
CGO_ENABLED=1 go build -o "$BUILD_DIR/$BINARY_NAME" "$SOURCE_FILE"

# 6. Verify build success
if [ $? -eq 0 ]; then
    echo -e "${GREEN}Build successful!${NC}"
    echo -e "Binary located at: ${GREEN}$BUILD_DIR/$BINARY_NAME${NC}"
    
    # Make the binary executable just in case
    chmod +x "$BUILD_DIR/$BINARY_NAME"
else
    echo -e "${RED}Build failed.${NC}"
    exit 1
fi

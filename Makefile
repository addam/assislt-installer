# ZOI Installer build helpers
# Requires: go 1.21+, python 3.11+

SERVER_URL ?= http://localhost:5000
# Override PUBLIC_KEY_HEX after running: python server/sign_installer.py pubkey
PUBLIC_KEY_HEX ?= 0000000000000000000000000000000000000000000000000000000000000000

BOOTSTRAPPER = server/get-artikulo.exe

.PHONY: keygen build-windows build-linux server sign

# Generate Ed25519 keypair.  Do this once; keep keys/private.key off the server.
keygen:
	cd server && python sign_installer.py keygen

# Build the Windows bootstrapper.exe and place it in server/ so Flask can serve it.
build-windows:
	cd bootstrapper && \
	  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	  go build \
	    -ldflags="-s -w -X main.ServerURL=$(SERVER_URL) -X main.PublicKeyHex=$(PUBLIC_KEY_HEX)" \
	    -o ../$(BOOTSTRAPPER) .
	@echo "Built $(BOOTSTRAPPER)"

# Build a native (Linux/macOS) binary for testing the flow locally.
build-linux:
	cd bootstrapper && \
	  go build \
	    -ldflags="-X main.ServerURL=$(SERVER_URL) -X main.PublicKeyHex=$(PUBLIC_KEY_HEX)" \
	    -o ../bootstrapper-test .
	@echo "Built bootstrapper-test"

# Sign a product installer (run after keygen).
# Usage:  make sign FILE=server/files/myapp-1.1.0.exe
sign:
	cd server && python sign_installer.py sign ../$(FILE)

# Run the development server.
server:
	cd server && flask --app app run --port 5000

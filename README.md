# Multi-version installer

Assislt-installer may be used to distribute various versions of various products to end users.
It is intended to share SmartScreen reputation across these.
Currently it is being used by ZOI UTIA to distribute versions of Assislt / Artikulo speech therapy tool, and further development preview products may follow.

##  Setup sequence
 1. Generate keypair (once)
```
  make keygen
```
   → prints public key hex

 2. Build the bootstrapper with your server URL and public key
```
  make build-windows \
    SERVER_URL=https://your.server \
    PUBLIC_KEY_HEX=<hex from step 1>
```
 → writes server/bootstrapper.exe

 3. Sign each product installer
```
  make sign FILE=server/files/myapp-1.1.0.exe
```
 → writes server/files/myapp-1.1.0.exe.sig

 4. Start the server
```
  make server
```

##  Flow recap

Single-product shortcut: if GET /products returns only one family, the bootstrapper picks the newest version and downloads directly — no browser step.

Multi-product OAuth-like flow:
 1. User visits the site → clicks Download → browser gets Set-Cookie: selected_product=<id> + downloads bootstrapper.exe
 2. User runs the .exe → it starts a random localhost port, opens the browser to /get-product?redirect_uri=http://127.0.0.1:<port>/callback&state=<nonce>
 3. Server reads the cookie, redirects back to localhost/callback?product_id=<id>&state=<nonce>
 4. Bootstrapper verifies the state nonce, looks up the product URL, downloads + verifies Ed25519 signature over SHA-256(installer), then launches it

The private key never leaves the signing machine; the public key is baked into the .exe at build time.

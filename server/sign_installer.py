#!/usr/bin/env python3
"""
sign_installer.py — Key management and installer signing for ZOI.

Commands:
  python sign_installer.py keygen          Generate a new Ed25519 keypair
  python sign_installer.py sign <file>     Sign <file>, writing <file>.sig
  python sign_installer.py pubkey          Print the public key hex (for bootstrapper build)

The private key lives in keys/private.key (keep it secret and off the server).
The signature is Ed25519( SHA-256(file_bytes) ).
The bootstrapper verifies with the public key baked in at build time.
"""

import hashlib
import os
import pathlib
import sys

try:
    from cryptography.hazmat.primitives.asymmetric.ed25519 import (
        Ed25519PrivateKey, Ed25519PublicKey,
    )
    from cryptography.hazmat.primitives.serialization import (
        Encoding, NoEncryption, PrivateFormat, PublicFormat,
    )
except ImportError:
    sys.exit("Install dependencies first:  pip install cryptography")

KEYS_DIR = pathlib.Path(__file__).parent / "keys"
PRIV_KEY_PATH = KEYS_DIR / "private.key"
PUB_KEY_PATH = KEYS_DIR / "public.key"


def keygen():
    KEYS_DIR.mkdir(exist_ok=True)

    private_key = Ed25519PrivateKey.generate()
    public_key = private_key.public_key()

    priv_bytes = private_key.private_bytes(Encoding.Raw, PrivateFormat.Raw, NoEncryption())
    PRIV_KEY_PATH.write_bytes(priv_bytes)
    PRIV_KEY_PATH.chmod(0o600)

    pub_bytes = public_key.public_bytes(Encoding.Raw, PublicFormat.Raw)
    PUB_KEY_PATH.write_bytes(pub_bytes)

    print(f"Private key: {PRIV_KEY_PATH}  (keep secret!)")
    print(f"Public key:  {PUB_KEY_PATH}")
    print()
    print("Public key hex — embed in bootstrapper build:")
    print(pub_bytes.hex())
    print()
    print("Build command:")
    hex_key = pub_bytes.hex()
    print(
        f"  cd bootstrapper && "
        f'GOOS=windows GOARCH=amd64 go build '
        f'-ldflags="-s -w -X main.ServerURL=https://your.server '
        f'-X main.PublicKeyHex={hex_key}" '
        f"-o ../server/bootstrapper.exe ."
    )


def sign(filepath: str):
    if not PRIV_KEY_PATH.exists():
        sys.exit(f"Private key not found at {PRIV_KEY_PATH}. Run keygen first.")

    priv_bytes = PRIV_KEY_PATH.read_bytes()
    private_key = Ed25519PrivateKey.from_private_bytes(priv_bytes)

    file_bytes = pathlib.Path(filepath).read_bytes()
    file_hash = hashlib.sha256(file_bytes).digest()
    signature = private_key.sign(file_hash)

    sig_path = filepath + ".sig"
    pathlib.Path(sig_path).write_bytes(signature)
    print(f"Signed:    {filepath}")
    print(f"Signature: {sig_path}")


def pubkey():
    if not PUB_KEY_PATH.exists():
        sys.exit(f"Public key not found at {PUB_KEY_PATH}. Run keygen first.")
    print(PUB_KEY_PATH.read_bytes().hex())


COMMANDS = {"keygen": keygen, "pubkey": pubkey}

if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] in COMMANDS:
        COMMANDS[sys.argv[1]]()
    elif len(sys.argv) == 3 and sys.argv[1] == "sign":
        sign(sys.argv[2])
    else:
        print(__doc__)
        sys.exit(1)

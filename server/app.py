"""
ZOI Installer Server

Provides:
  GET  /products           — JSON list of available products
  GET  /download/<id>      — serves bootstrapper.exe + sets selected_product cookie
  GET  /get-product        — reads cookie, redirects to bootstrapper's localhost listener

Development file serving (put files in ./files/):
  GET  /files/<filename>   — serves product installers and .sig files
"""

import json
import pathlib
from urllib.parse import urlparse

from flask import (Flask, abort, jsonify, make_response,
                   redirect, request, send_from_directory)

app = Flask(__name__)

BASE = pathlib.Path(__file__).parent
PRODUCTS_FILE = BASE / "products.json"
BOOTSTRAPPER_PATH = BASE / "bootstrapper.exe"
FILES_DIR = BASE / "files"


def load_products() -> list[dict]:
    with open(PRODUCTS_FILE) as f:
        return json.load(f)


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

@app.get("/products")
def get_products():
    """Return the full product catalogue."""
    return jsonify(load_products())


@app.get("/download")
@app.get("/download/<product_id>")
def download(product_id: str | None = None):
    """
    Serve bootstrapper.exe.  If product_id is provided, set a short-lived
    cookie so that /get-product can redirect back to the bootstrapper with
    the right choice without requiring the user to log in.
    """
    if not BOOTSTRAPPER_PATH.exists():
        abort(503, "Bootstrapper binary not yet built; run `make build-windows`")

    if product_id is not None:
        products = load_products()
        if not any(p["id"] == product_id for p in products):
            abort(404, f"Unknown product id: {product_id!r}")

    response = make_response(
        send_from_directory(
            BASE, "bootstrapper.exe",
            as_attachment=True,
            download_name="installer.exe",
            mimetype="application/octet-stream",
        )
    )

    if product_id is not None:
        response.set_cookie(
            "selected_product",
            product_id,
            max_age=3600,       # user has 1 hour to run the downloaded exe
            httponly=True,
            samesite="Lax",
            secure=False,       # set to True when deploying over HTTPS
        )

    return response


@app.get("/get-product")
def get_product():
    """
    OAuth-like redirect endpoint.

    The bootstrapper opens the browser here with:
      ?redirect_uri=http://127.0.0.1:<port>/callback&state=<random>

    This endpoint reads the selected_product cookie (set during download) and
    redirects to:
      redirect_uri?product_id=<id>&state=<state>

    The bootstrapper's local HTTP server handles the callback and extracts
    product_id, then looks up the download URL in the /products list it
    already fetched.
    """
    redirect_uri = request.args.get("redirect_uri", "")
    state = request.args.get("state", "")

    if not redirect_uri:
        abort(400, "redirect_uri is required")

    # Restrict redirect_uri to localhost to prevent open-redirect abuse.
    try:
        parsed = urlparse(redirect_uri)
        if parsed.hostname not in ("localhost", "127.0.0.1", "::1"):
            abort(400, "redirect_uri must target localhost")
    except Exception:
        abort(400, "Malformed redirect_uri")

    product_id = request.cookies.get("selected_product")
    if not product_id:
        return (
            "<html><body>"
            "<h2>Session expired</h2>"
            "<p>Please go back to the website and click <em>Download</em> again.</p>"
            "</body></html>"
        ), 400

    params = {"product_id": product_id}
    if state:
        params["state"] = state

    sep = "&" if "?" in redirect_uri else "?"
    query = "&".join(f"{k}={v}" for k, v in params.items())
    return redirect(f"{redirect_uri}{sep}{query}")


# ---------------------------------------------------------------------------
# Development file server (not needed in production with a CDN)
# ---------------------------------------------------------------------------

@app.get("/files/<path:filename>")
def serve_file(filename: str):
    if not FILES_DIR.exists():
        abort(404)
    return send_from_directory(FILES_DIR, filename)


if __name__ == "__main__":
    app.run(debug=True, port=5000)

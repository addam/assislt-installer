// zoi-bootstrapper: downloads and verifies a signed product installer,
// sharing SmartScreen filter reputation across many different product installers.
//
// Build for Windows:
//   GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.ServerURL=https://your.server -X main.PublicKeyHex=<hex>" -o ../server/bootstrapper.exe .
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Overridden at build time via -ldflags="-X main.ServerURL=... -X main.PublicKeyHex=..."
var ServerURL = "http://localhost:5000"
var PublicKeyHex = "0000000000000000000000000000000000000000000000000000000000000000"

type Product struct {
	ID          string `json:"id"`
	Family      string `json:"family"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SigURL      string `json:"sig_url"`
}

func main() {
	products, err := fetchProducts()
	if err != nil {
		fatal("Failed to fetch products: %v", err)
	}
	if len(products) == 0 {
		fatal("No products available from server")
	}

	var chosen *Product

	if isSingleFamily(products) {
		// Shortcut: all entries are versions of one product — download newest directly.
		chosen = newestInFamily(products)
		fmt.Printf("Downloading %s %s...\n", chosen.Name, chosen.Version)
	} else {
		// OAuth-like flow: open browser so the server can read the cookie it set
		// when the user clicked the download button, and redirect back here.
		chosen, err = oauthFlow(products)
		if err != nil {
			fatal("Product selection failed: %v", err)
		}
		fmt.Printf("Downloading %s %s...\n", chosen.Name, chosen.Version)
	}

	installerPath, err := downloadAndVerify(chosen)
	if err != nil {
		fatal("Download/verify failed: %v", err)
	}

	fmt.Println("Launching installer...")
	if err := launchInstaller(installerPath); err != nil {
		os.Remove(installerPath)
		fatal("Could not launch installer: %v", err)
	}
}

// fetchProducts calls GET /products on the server.
func fetchProducts() ([]Product, error) {
	resp, err := http.Get(ServerURL + "/products")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}
	var products []Product
	if err := json.NewDecoder(resp.Body).Decode(&products); err != nil {
		return nil, fmt.Errorf("decoding product list: %w", err)
	}
	return products, nil
}

// isSingleFamily returns true when every product shares the same family name.
func isSingleFamily(products []Product) bool {
	family := products[0].Family
	for _, p := range products[1:] {
		if p.Family != family {
			return false
		}
	}
	return true
}

// newestInFamily returns the product with the highest version string.
func newestInFamily(products []Product) *Product {
	sorted := make([]Product, len(products))
	copy(sorted, products)
	sort.Slice(sorted, func(i, j int) bool {
		return cmpVersion(sorted[i].Version, sorted[j].Version) < 0
	})
	return &sorted[len(sorted)-1]
}

// cmpVersion compares two dot-separated version strings numerically.
// Returns -1, 0, or 1.
func cmpVersion(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(ap) {
			av, _ = strconv.Atoi(ap[i])
		}
		if i < len(bp) {
			bv, _ = strconv.Atoi(bp[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// oauthFlow starts a temporary localhost HTTP server, opens the browser to the
// server's /get-product endpoint (which reads the cookie set at download time),
// and waits for the server to redirect back with the chosen product_id.
func oauthFlow(products []Product) (*Product, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not start local server: %w", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Random state prevents a rogue local service from injecting a product_id.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, err
	}
	state := hex.EncodeToString(stateBytes)

	type callbackResult struct {
		productID string
		err       error
	}
	resultCh := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			resultCh <- callbackResult{err: fmt.Errorf("state mismatch in callback")}
			return
		}
		productID := r.URL.Query().Get("product_id")
		if productID == "" {
			http.Error(w, "missing product_id", http.StatusBadRequest)
			resultCh <- callbackResult{err: fmt.Errorf("no product_id in callback")}
			return
		}
		fmt.Fprint(w, `<html><body><h2>Download started</h2><p>You can close this tab.</p></body></html>`)
		resultCh <- callbackResult{productID: productID}
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln) //nolint:errcheck

	authURL := ServerURL + "/get-product?" + url.Values{
		"redirect_uri": {redirectURI},
		"state":        {state},
	}.Encode()

	fmt.Println("Opening browser to select product...")
	if err := openBrowser(authURL); err != nil {
		srv.Shutdown(context.Background()) //nolint:errcheck
		return nil, fmt.Errorf("could not open browser: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	select {
	case res := <-resultCh:
		srv.Shutdown(context.Background()) //nolint:errcheck
		if res.err != nil {
			return nil, res.err
		}
		for i, p := range products {
			if p.ID == res.productID {
				return &products[i], nil
			}
		}
		return nil, fmt.Errorf("product %q not found in server product list", res.productID)
	case <-ctx.Done():
		srv.Shutdown(context.Background()) //nolint:errcheck
		return nil, fmt.Errorf("timed out waiting for product selection (5 min)")
	}
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

// downloadAndVerify fetches the installer and its Ed25519 signature, verifies
// the signature against SHA-256(installer), and returns a path to the temp file.
func downloadAndVerify(p *Product) (string, error) {
	pubKeyBytes, err := hex.DecodeString(PublicKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid embedded public key")
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	fmt.Printf("  Downloading installer from %s\n", p.DownloadURL)
	installerPath, hash, err := downloadToTempFile(p.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading installer: %w", err)
	}

	fmt.Printf("  Downloading signature from %s\n", p.SigURL)
	sigData, err := downloadToMemory(p.SigURL)
	if err != nil {
		os.Remove(installerPath)
		return "", fmt.Errorf("downloading signature: %w", err)
	}

	if !ed25519.Verify(pubKey, hash[:], sigData) {
		os.Remove(installerPath)
		return "", fmt.Errorf("signature verification FAILED — installer may be tampered with")
	}
	fmt.Println("  Signature OK.")
	return installerPath, nil
}

// downloadToTempFile streams a URL to a .exe temp file and returns its path
// along with the SHA-256 digest of its contents.
func downloadToTempFile(rawURL string) (path string, hash [32]byte, err error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return "", hash, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", hash, fmt.Errorf("server returned %s", resp.Status)
	}

	f, err := os.CreateTemp("", "zoi-installer-*.exe")
	if err != nil {
		return "", hash, err
	}

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", hash, err
	}
	f.Close()
	copy(hash[:], h.Sum(nil))
	return f.Name(), hash, nil
}

func downloadToMemory(rawURL string) ([]byte, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// launchInstaller runs the installer and waits for it to exit, then removes
// the temp file.  The installer may itself spawn sub-processes (e.g. NSIS),
// so we wait only for the top-level process.
func launchInstaller(path string) error {
	cmd := exec.Command(path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	os.Remove(path) // best-effort cleanup
	return err
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

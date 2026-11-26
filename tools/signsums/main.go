// Command signsums makes the optional ed25519 signature for a release's
// SHA256SUMS.txt, so Rover builds that carry the matching public key
// (main.updatePublicKey) refuse unsigned or altered releases.
//
//	go run ./tools/signsums -keygen            # prints a new key pair (once)
//	UPDATE_SIGNING_KEY=<private> go run ./tools/signsums SHA256SUMS.txt
//
// The private key is the base64 32-byte seed; keep it in a GitHub secret.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	keygen := flag.Bool("keygen", false, "print a new key pair and exit")
	flag.Parse()
	if *keygen {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			fail(err)
		}
		fmt.Println("UPDATE_SIGNING_KEY (secret, keep private):", base64.StdEncoding.EncodeToString(priv.Seed()))
		fmt.Println("UPDATE_PUBLIC_KEY  (public, a repository variable):", base64.StdEncoding.EncodeToString(pub))
		return
	}
	if flag.NArg() != 1 {
		fail(fmt.Errorf("usage: signsums [-keygen] SHA256SUMS.txt"))
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("UPDATE_SIGNING_KEY")))
	if err != nil || len(seed) != ed25519.SeedSize {
		fail(fmt.Errorf("UPDATE_SIGNING_KEY must be the base64 32-byte seed printed by -keygen"))
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fail(err)
	}
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), data)
	if err := os.WriteFile(flag.Arg(0)+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		fail(err)
	}
	fmt.Println("wrote", flag.Arg(0)+".sig")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "signsums:", err)
	os.Exit(1)
}

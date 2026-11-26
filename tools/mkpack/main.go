// mkpack downloads and verifies the offline brain files and builds Rover
// offline packs (.dlpack, split into parts under 2 GB) or a staging folder
// for the "Rover Offline (full)" installer.
//
//	go run ./tools/mkpack -cache dl -model qwen3-1.7b -runtimes cpu,vulkan -out dist/Rover-Offline-Light.dlpack
//	go run ./tools/mkpack -cache dl -model qwen3-1.7b -runtimes cpu,vulkan -stage installer/offline
//
// Every file is checked against catalog.json (the same SHA-256 values the
// app has built in); a mismatch stops the build.
package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"hostpanel/dlpack"
)

//go:embed catalog.json
var catalogJSON []byte

type entry struct {
	ID      string `json:"id"`
	Short   string `json:"short"`
	File    string `json:"file"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	License string `json:"license"`
}

type catalog struct {
	Build       string  `json:"build"`
	RuntimeBase string  `json:"runtime_base"`
	Models      []entry `json:"models"`
	Runtimes    []entry `json:"runtimes"`
}

func main() {
	cache := flag.String("cache", "mkpack-cache", "download folder (reused between runs)")
	model := flag.String("model", "qwen3-1.7b", "model id (or none)")
	rts := flag.String("runtimes", "cpu,vulkan", "llama.cpp builds to include")
	out := flag.String("out", "", "pack file to write (.dlpack); split into .001/.002 when larger than -part")
	stage := flag.String("stage", "", "copy the verified files into this folder instead (installer staging)")
	part := flag.Int64("part", dlpack.DefaultPart, "maximum part size in bytes")
	offline := flag.Bool("offline", false, "do not download; files must already be in -cache")
	flag.Parse()
	var cat catalog
	if err := json.Unmarshal(catalogJSON, &cat); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*cache, 0o755); err != nil {
		log.Fatal(err)
	}
	var srcs []dlpack.Source
	title := "Rover offline brain"
	if *model != "none" && *model != "" {
		e := find(cat.Models, *model)
		if e == nil {
			log.Fatalf("unknown model %q", *model)
		}
		p := fetch(*cache, *e, e.URL, *offline)
		srcs = append(srcs, dlpack.Source{Path: p, File: dlpack.File{Name: e.File, Kind: "model", ID: e.ID, SHA256: e.SHA256, License: e.License}})
		title += " - " + e.Short
	}
	for _, id := range strings.Split(*rts, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		e := find(cat.Runtimes, id)
		if e == nil {
			log.Fatalf("unknown runtime %q", id)
		}
		p := fetch(*cache, *e, cat.RuntimeBase+e.File, *offline)
		srcs = append(srcs, dlpack.Source{Path: p, File: dlpack.File{Name: e.File, Kind: "runtime", ID: e.ID, SHA256: e.SHA256, License: e.License}})
	}
	type res struct {
		File string `json:"file"`
		Size int64  `json:"size"`
		SHA  string `json:"sha256"`
	}
	var done []res
	switch {
	case *stage != "":
		if err := os.MkdirAll(*stage, 0o755); err != nil {
			log.Fatal(err)
		}
		for _, s := range srcs {
			dst := filepath.Join(*stage, s.Name)
			if err := copyFile(s.Path, dst); err != nil {
				log.Fatal(err)
			}
			st, _ := os.Stat(dst)
			done = append(done, res{dst, st.Size(), s.SHA256})
		}
	case *out != "":
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			log.Fatal(err)
		}
		files, err := dlpack.Write(*out, title, cat.Build, srcs, *part, nil)
		if err != nil {
			log.Fatal(err)
		}
		for _, f := range files {
			st, _ := os.Stat(f)
			if st.Size() >= 2<<30 {
				log.Fatalf("%s is %d bytes: over GitHub's 2 GiB asset limit", f, st.Size())
			}
			done = append(done, res{f, st.Size(), sum(f)})
		}
		if p, err := dlpack.Open(files[0]); err != nil {
			log.Fatalf("the written pack does not open: %v", err)
		} else {
			p.Close()
		}
	default:
		log.Fatal("give -out or -stage")
	}
	b, _ := json.MarshalIndent(done, "", "  ")
	fmt.Println(string(b))
}

func find(es []entry, id string) *entry {
	for i := range es {
		if es[i].ID == id {
			return &es[i]
		}
	}
	return nil
}

func sum(p string) string {
	f, err := os.Open(p)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func fetch(dir string, e entry, url string, offline bool) string {
	dst := filepath.Join(dir, e.File)
	if st, err := os.Stat(dst); err == nil && st.Size() == e.Size && sum(dst) == e.SHA256 {
		log.Printf("ok (cached) %s", e.File)
		return dst
	}
	if offline {
		log.Fatalf("%s is missing or wrong in %s", e.File, dir)
	}
	log.Printf("downloading %s (%d bytes)", e.File, e.Size)
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("%s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dst + ".part")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		log.Fatal(err)
	}
	f.Close()
	if s := sum(dst + ".part"); s != e.SHA256 {
		os.Remove(dst + ".part")
		log.Fatalf("%s: checksum mismatch (got %s, want %s)", e.File, s, e.SHA256)
	}
	if err := os.Rename(dst+".part", dst); err != nil {
		log.Fatal(err)
	}
	log.Printf("ok %s", e.File)
	return dst
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

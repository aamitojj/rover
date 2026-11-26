// Package dlpack reads and writes Rover offline packs: an uncompressed
// zip holding manifest.json plus the files (llama.cpp engine zips and a
// GGUF model). Packs larger than one part are split byte-wise into
// name.dlpack.001, .002, ... so every part stays under GitHub's 2 GiB
// release-asset limit (and FAT32's 4 GiB file limit on USB sticks).
//
// The manifest is informational only: the app accepts a file only when its
// SHA-256 matches its own pinned catalogue.
package dlpack

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Format = "dilreet-pack/1"

// DefaultPart keeps parts below 2 GiB with room to spare.
const DefaultPart int64 = 2_000_000_000

type File struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`         // model | runtime
	ID      string `json:"id,omitempty"` // model id or runtime kind (cpu | vulkan)
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	License string `json:"license,omitempty"`
}

type Manifest struct {
	Format  string `json:"format"`
	Build   string `json:"build,omitempty"`
	Created string `json:"created"`
	Title   string `json:"title,omitempty"`
	Files   []File `json:"files"`
}

// Source is a file to put in a pack.
type Source struct {
	File
	Path string
}

// Write creates base (e.g. "out/Rover-Offline-Light.dlpack"); when the
// result is larger than part bytes it is split into base.001, base.002 ...
// It returns the files written.
func Write(base string, title, build string, srcs []Source, part int64, progress func(done, total int64)) ([]string, error) {
	if part <= 0 {
		part = DefaultPart
	}
	m := Manifest{Format: Format, Build: build, Created: time.Now().UTC().Format(time.RFC3339), Title: title}
	var total int64
	for _, s := range srcs {
		st, err := os.Stat(s.Path)
		if err != nil {
			return nil, err
		}
		f := s.File
		f.Size = st.Size()
		if f.Name == "" {
			f.Name = filepath.Base(s.Path)
		}
		m.Files = append(m.Files, f)
		total += f.Size
	}
	sw := &splitWriter{base: base, part: part}
	zw := zip.NewWriter(sw)
	mb, _ := json.MarshalIndent(m, "", "  ")
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Store, Modified: time.Now()})
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(mb); err != nil {
		return nil, err
	}
	var done int64
	for i, s := range srcs {
		f := m.Files[i]
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Store, Modified: time.Now()})
		if err != nil {
			sw.Close()
			return nil, err
		}
		in, err := os.Open(s.Path)
		if err != nil {
			sw.Close()
			return nil, err
		}
		h := sha256.New()
		_, err = io.Copy(io.MultiWriter(w, h, progressWriter{&done, total, progress}), in)
		in.Close()
		if err != nil {
			sw.Close()
			return nil, err
		}
		if f.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), f.SHA256) {
			sw.Close()
			return nil, fmt.Errorf("%s does not match its checksum", f.Name)
		}
	}
	if err := zw.Close(); err != nil {
		sw.Close()
		return nil, err
	}
	if err := sw.Close(); err != nil {
		return nil, err
	}
	return sw.finish()
}

type progressWriter struct {
	done     *int64
	total    int64
	progress func(done, total int64)
}

func (p progressWriter) Write(b []byte) (int, error) {
	*p.done += int64(len(b))
	if p.progress != nil {
		p.progress(*p.done, p.total)
	}
	return len(b), nil
}

// splitWriter writes base.001, base.002, ... then renames to base when a
// single part was enough.
type splitWriter struct {
	base  string
	part  int64
	cur   *os.File
	n     int64
	names []string
}

func (s *splitWriter) Write(b []byte) (int, error) {
	written := 0
	for len(b) > 0 {
		if s.cur == nil || s.n >= s.part {
			if s.cur != nil {
				if err := s.cur.Close(); err != nil {
					return written, err
				}
			}
			name := fmt.Sprintf("%s.%03d", s.base, len(s.names)+1)
			f, err := os.Create(name)
			if err != nil {
				return written, err
			}
			s.cur, s.n, s.names = f, 0, append(s.names, name)
		}
		k := int64(len(b))
		if room := s.part - s.n; k > room {
			k = room
		}
		m, err := s.cur.Write(b[:k])
		written += m
		s.n += int64(m)
		if err != nil {
			return written, err
		}
		b = b[k:]
	}
	return written, nil
}

func (s *splitWriter) Close() error {
	if s.cur == nil {
		return nil
	}
	err := s.cur.Close()
	s.cur = nil
	return err
}

func (s *splitWriter) finish() ([]string, error) {
	if len(s.names) == 1 {
		if err := os.Rename(s.names[0], s.base); err != nil {
			return nil, err
		}
		return []string{s.base}, nil
	}
	return s.names, nil
}

// ---------- reading ----------

type Pack struct {
	Manifest Manifest
	Parts    []string
	Size     int64
	zr       *zip.Reader
	files    []*os.File
}

// Parts finds every part of a pack: "x.dlpack" alone, or "x.dlpack.001"
// (any part given) plus its siblings.
func Parts(path string) ([]string, error) {
	ext := filepath.Ext(path)
	if len(ext) == 4 && strings.Trim(ext[1:], "0123456789") == "" {
		base := strings.TrimSuffix(path, ext)
		var out []string
		for i := 1; i < 1000; i++ {
			p := fmt.Sprintf("%s.%03d", base, i)
			if _, err := os.Stat(p); err != nil {
				break
			}
			out = append(out, p)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s.001 is missing", filepath.Base(base))
		}
		return out, nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

// Find looks for packs in a folder (e.g. a USB stick or the app's offline
// folder); only the first part of split packs is returned.
func Find(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(n), ".dlpack") || strings.HasSuffix(strings.ToLower(n), ".dlpack.001") {
			out = append(out, filepath.Join(dir, n))
		}
	}
	sort.Strings(out)
	return out
}

func Open(path string) (*Pack, error) {
	parts, err := Parts(path)
	if err != nil {
		return nil, err
	}
	p := &Pack{Parts: parts}
	var ra multiReaderAt
	for _, n := range parts {
		f, err := os.Open(n)
		if err != nil {
			p.Close()
			return nil, err
		}
		st, _ := f.Stat()
		p.files = append(p.files, f)
		ra.parts = append(ra.parts, partAt{f, ra.size, st.Size()})
		ra.size += st.Size()
	}
	p.Size = ra.size
	zr, err := zip.NewReader(&ra, ra.size)
	if err != nil {
		p.Close()
		if len(parts) > 1 {
			return nil, errors.New("this is not a Rover offline pack, or a part is missing or incomplete")
		}
		return nil, errors.New("this is not a Rover offline pack (or the file is incomplete)")
	}
	p.zr = zr
	rc, err := p.open("manifest.json")
	if err != nil {
		p.Close()
		return nil, errors.New("this pack has no manifest.json")
	}
	err = json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&p.Manifest)
	rc.Close()
	if err != nil || p.Manifest.Format != Format {
		p.Close()
		return nil, errors.New("this pack is in an unknown format; update Rover")
	}
	return p, nil
}

func (p *Pack) open(name string) (io.ReadCloser, error) {
	for _, f := range p.zr.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, os.ErrNotExist
}

// Extract copies one file out while hashing it; the caller compares the
// returned hash with its own pinned value before using the file.
func (p *Pack) Extract(name, dst string, progress func(n int64)) (string, int64, error) {
	rc, err := p.open(name)
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	var n int64
	buf := make([]byte, 1<<20)
	for {
		k, rerr := rc.Read(buf)
		if k > 0 {
			if _, err := out.Write(buf[:k]); err != nil {
				out.Close()
				os.Remove(tmp)
				return "", 0, err
			}
			h.Write(buf[:k])
			n += int64(k)
			if progress != nil {
				progress(n)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			os.Remove(tmp)
			return "", 0, rerr
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, os.Rename(tmp, dst)
}

func (p *Pack) Close() {
	for _, f := range p.files {
		f.Close()
	}
	p.files = nil
}

type partAt struct {
	f    *os.File
	off  int64
	size int64
}

type multiReaderAt struct {
	parts []partAt
	size  int64
}

func (m *multiReaderAt) ReadAt(b []byte, off int64) (int, error) {
	if off >= m.size {
		return 0, io.EOF
	}
	n := 0
	for _, p := range m.parts {
		if len(b) == 0 {
			break
		}
		if off >= p.off+p.size || off < p.off {
			continue
		}
		k, err := p.f.ReadAt(b[:min(int64(len(b)), p.off+p.size-off)], off-p.off)
		n += k
		off += int64(k)
		b = b[k:]
		if err != nil && err != io.EOF {
			return n, err
		}
	}
	if len(b) > 0 {
		return n, io.EOF
	}
	return n, nil
}

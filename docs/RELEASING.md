# Publishing a Rover update

Every Rover copy checks the **GitHub Releases** of `aamitojj/rover` (unless the user
changed the repository under *Settings → Updates → Advanced*). When you publish a newer
release, people see **Update available** and can install it with one click, or it installs
by itself if they chose that. You do not need to host anything else.

## The easy way: one button on GitHub (no git commands)

1. Make sure the code you want to release is on the `main` branch.
2. On GitHub open **Actions → Release → Run workflow**.
3. Type the new version, for example `0.8.1`. It must be higher than the current one.
   * Tick **Beta / pre-release** for a test version (for example `0.7.0-beta.1`). Only
     people who chose the *Beta* channel are offered it.
   * Optionally type a short **What's new**. People see it before they update. GitHub
     also adds the list of changes automatically.
4. Press **Run workflow**. In about 20 minutes the release `v0.8.1` appears under
   **Releases** with these files:
   * `Rover-Setup-0.8.1.exe` (installed copies update with this)
   * `rover.exe` (portable copies update with this)
   * `Rover-Offline-Setup-0.8.1.exe` plus `Rover-Offline-Setup-0.8.1-1.bin`, `-2.bin`… (the
     installer with Jiv's Light brain built in, split into parts under 2 GB; keep the parts
     in one folder and run the `.exe`). The updater never picks these.
   * `Rover-Offline-Light-0.8.1.dlpack` (or `.dlpack.001`, `.002`… parts) and, if enabled,
     `Rover-Offline-Standard-0.8.1.dlpack…`: offline packs for *Settings → Jiv → Import offline pack*.
   * `Dilreet-Setup-0.8.1.exe` and `dilreet.exe`: copies under the old names so Dilreet 0.6/0.7
     installations (which look for those names) can update to Rover. Keep publishing them for
     a few releases.
   * `SHA256SUMS.txt` (fingerprints of every file; Rover refuses any file that does not match)

That's all. Running copies find it within a day (at start-up or daily), or straight away
with *Settings → Updates → Check now*.

## The git way

```
git tag v0.8.1
git push origin v0.8.1
```

Pushing a tag that starts with `v` runs the same build and publishes the release.
A version with a dash, like `v0.7.0-beta.1`, is published as a pre-release.

## If something goes wrong

* **The workflow says the version already exists:** choose a higher number.
* **A bad release went out:** on GitHub, edit the release and tick *Set as a pre-release*
  (stable users stop seeing it) or delete it, then publish a fixed, higher version.
  Users can always roll back by reinstalling an older `Rover-Setup-x.y.z.exe` from Releases;
  their settings and projects are kept.
* **People can't download:** GitHub allows 60 update checks per hour per internet
  connection without an account. Rover checks at most about once a day, so this is
  only hit when many PCs share one connection; it then says "try again later".

## Optional: signed releases (extra protection)

SHA256SUMS.txt protects against broken or swapped downloads. If you also want to be sure a
release was made by you (even if someone got access to the GitHub repository), add a
signature. Do this once:

1. On any PC with Go: `go run ./tools/signsums -keygen`. It prints two lines.
2. In GitHub: *Settings → Secrets and variables → Actions*:
   * **Secrets → New repository secret** `UPDATE_SIGNING_KEY` = the first (private) value.
   * **Variables → New repository variable** `UPDATE_PUBLIC_KEY` = the second (public) value.
3. Publish the next release as usual. It now also has `SHA256SUMS.txt.sig`, and the
   `rover.exe` inside carries the public key.

From then on, copies built with the key refuse any update that is not signed with the
matching private key. Keep the private key safe: if you lose it, people must install the
next version by hand. Copies built before you added the key (or without it) keep working
with SHA256SUMS.txt only.

## What the build does (for reference)

`.github/workflows/build.yml` runs the tests (gofmt, vet for Linux and Windows,
`go test -race`, `node --check`), then on Windows builds `rover.exe` with the version
stamped in (`-X main.Version=…`), builds the Inno Setup installer, writes `SHA256SUMS.txt`
(and the signature if the secret exists) and, for tags or the Release button, publishes them
with `softprops/action-gh-release`. The `offline` job downloads the pinned model and llama.cpp
engines (sha256-checked), stages them with `go run ./tools/mkpack`, writes the `.dlpack` packs, builds
`iscc /DOffline=Light` and fails if any single file is 2 GiB or more. The `publish` job adds the
`Dilreet-*` copies, then writes `SHA256SUMS.txt` over everything. `.github/workflows/release.yml` checks the version and
then calls build.yml.

Rover reads releases from `https://api.github.com/repos/<owner>/<repo>/releases/latest`
(stable) or `/releases` (beta), so draft releases are never offered.

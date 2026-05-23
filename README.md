# patchwork

<img src="logo.svg" alt="Patchwork logo" width="100" align="left" hspace="14" />

A static site generator for making beautiful group and office greeting cards
for loved ones.

[Example demo site](https://aae42.github.io/patchwork/).

It's like [kudoboard](https://www.kudoboard.com/)
(which it draws heavy inspiration from) but for nerds who can contribute
markdown to a git repository.

## What It Does

`patchwork` is a command line tool that reads a single board directory of
markdown documents and builds a static web page at `public/index.html` by
default.

The generated page:

- Shows a grid of all card content
- Displays author, title, and optional image
- Lets you click a card to open a larger modal view

## Board Config

Create or edit `patchwork.yaml` ([example](examples/boards/celebration/patchwork.yaml))
inside the board directory to set page text:

```yaml
page:
  title: Team Tribute Board
  subtitle: A collection of notes from everyone cheering you on.

footer:
  enabled: true
```

If `patchwork.yaml` is missing in the board directory, default page text is used.
The footer is enabled by default. Set `footer.enabled: false` to hide it.

## Input Format

The input argument must point to one board directory (for example,
[`./examples/boards/celebration`](examples/boards/celebration)). Markdown files are read from that directory
only.

Each markdown file can include YAML front matter. `author` is supported now,
with optional `title` and `image`.

Each post is limited to 3000 characters of markdown content after front matter.
If a post goes over that limit, the CLI will report a file-specific error.

```md
---
author: Alex Rivera
title: A Big Congratulations
image: images/confetti.svg
---

Your markdown content goes here.
```

### Image Conventions

Set `image` in front matter to a path relative to that markdown file.

Supported formats: `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg`.

All image assets are copied into `<output-dir>/assets/`.

## Run

```bash
go run . ./examples/boards/celebration
```

Optional output directory:

```bash
go run . ./examples/boards/celebration ./site
```

After running, open `public/index.html` in your browser (or `<output-dir>/index.html`
if you provided an output directory).

## Deploy A Board to GitHub Pages

This repo includes a workflow at [`.github/workflows/deploy-example-board.yml`](.github/workflows/deploy-example-board.yml)
that builds [`examples/boards/celebration`](examples/boards/celebration) and
deploys `public/` to GitHub Pages.

1. Push this repo to GitHub.
2. In GitHub, open **Settings -> Pages**.
3. Set **Source** to **GitHub Actions**.
4. Push to `main` (or run the workflow manually from **Actions**).

After the workflow completes, your board is published.

See the example board here: <https://aae42.github.io/patchwork/>

## Releases

This repo includes a Goreleaser configuration at [.goreleaser.yaml](.goreleaser.yaml)
and a tag-triggered workflow at [.github/workflows/release.yml](.github/workflows/release.yml).

To cut a release, create and push a version tag:

```bash
git tag "v$(cat VERSION)"
git push origin "v$(cat VERSION)"
```

GitHub Actions will build release archives and checksums for macOS, Linux, and
Windows.

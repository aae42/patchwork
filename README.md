# patchwork

A static site generator for making beautiful group and office greeting cards
for loved ones.

## What It Does

`patchwork` is a command line tool that reads a single board directory of
markdown documents and builds a static web page at `public/index.html` by
default.

The generated page:

- Shows a grid of all card content
- Displays author, title, and optional image
- Lets you click a card to open a larger modal view

## Board Config

Create or edit `patchwork.yaml` inside the board directory to set page text:

```yaml
page:
  title: Team Tribute Board
  subtitle: A collection of notes from everyone cheering you on.
```

If `patchwork.yaml` is missing in the board directory, default page text is used.

## Input Format

The input argument must point to one board directory (for example,
`./examples/boards/celebration`). Markdown files are read from that directory
only.

Each markdown file can include YAML front matter. `author` is supported now,
with optional `title` and `image`.

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

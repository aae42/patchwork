package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

type frontMatter struct {
	Author string `yaml:"author"`
	Image  string `yaml:"image"`
	Title  string `yaml:"title"`
}

type card struct {
	ID          string
	Title       string
	Author      string
	HTML        template.HTML
	PreviewHTML template.HTML
	Image       string
	ThumbImage  string
}

type pageData struct {
	PageTitle     string
	PageSubtitle  string
	FooterEnabled bool
	Cards         []card
}

type appConfig struct {
	Page   pageConfig   `yaml:"page"`
	Footer footerConfig `yaml:"footer"`
}

type pageConfig struct {
	Title    string `yaml:"title"`
	Subtitle string `yaml:"subtitle"`
}

type footerConfig struct {
	Enabled bool `yaml:"enabled"`
}

// version is set at build time via -ldflags.
var version = "dev"

//go:embed logo.svg
var defaultLogoSVG []byte

const maxPostCharacters = 3000

const (
	thumbMaxWidth  = 640
	thumbMaxHeight = 420
	thumbJPEGQ     = 80
)

func defaultConfig() appConfig {
	return appConfig{
		Page: pageConfig{
			Title:    "Patchwork Cards",
			Subtitle: "Each tile is a stitched-together note from the people who care about you most.",
		},
		Footer: footerConfig{
			Enabled: true,
		},
	}
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(version)
		return
	}

	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <path-to-board-dir> [output-dir]\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	boardDir, err := filepath.Abs(os.Args[1])
	if err != nil {
		exitErr(fmt.Errorf("resolve board directory: %w", err))
	}

	info, err := os.Stat(boardDir)
	if err != nil {
		exitErr(fmt.Errorf("read board directory: %w", err))
	}
	if !info.IsDir() {
		exitErr(errors.New("board path must be a directory"))
	}

	config, err := loadConfig(filepath.Join(boardDir, "patchwork.yaml"))
	if err != nil {
		exitErr(err)
	}

	outputDir := "public"
	if len(os.Args) == 3 {
		outputDir = strings.TrimSpace(os.Args[2])
		if outputDir == "" {
			exitErr(errors.New("output directory cannot be empty"))
		}
	}
	if err := os.RemoveAll(outputDir); err != nil {
		exitErr(fmt.Errorf("reset output directory: %w", err))
	}
	if err := os.MkdirAll(filepath.Join(outputDir, "assets"), 0o755); err != nil {
		exitErr(fmt.Errorf("create output directories: %w", err))
	}
	if err := writeFavicon(outputDir); err != nil {
		exitErr(fmt.Errorf("write favicon: %w", err))
	}

	cards, err := loadCards(boardDir, outputDir)
	if err != nil {
		exitErr(err)
	}
	if len(cards) == 0 {
		exitErr(errors.New("no markdown files found in board directory"))
	}

	if err := writeIndexHTML(filepath.Join(outputDir, "index.html"), config, cards); err != nil {
		exitErr(fmt.Errorf("write index.html: %w", err))
	}

	fmt.Printf("Built %d cards in %s\n", len(cards), filepath.Join(outputDir, "index.html"))
}

func loadCards(inputDir, outputDir string) ([]card, error) {
	var cards []card

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			continue
		}

		path := filepath.Join(inputDir, entry.Name())
		c, err := parseCard(path, inputDir, outputDir, len(cards)+1)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		cards = append(cards, c)
	}

	sort.Slice(cards, func(i, j int) bool {
		return cards[i].Title < cards[j].Title
	})

	return cards, nil
}

func parseCard(path, inputDir, outputDir string, index int) (card, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return card{}, err
	}

	meta, body, err := extractFrontMatter(raw)
	if err != nil {
		return card{}, err
	}

	if strings.TrimSpace(meta.Author) == "" {
		meta.Author = "Anonymous"
	}

	postCharacters := utf8.RuneCountInString(body)
	if postCharacters > maxPostCharacters {
		return card{}, fmt.Errorf("%s has %d characters, but the maximum allowed is %d. Shorten this post or split it into a new board.", path, postCharacters, maxPostCharacters)
	}

	htmlContent, err := markdownToHTML(body)
	if err != nil {
		return card{}, fmt.Errorf("render markdown: %w", err)
	}

	relMarkdownPath, err := filepath.Rel(inputDir, path)
	if err != nil {
		return card{}, err
	}

	imagePath, err := resolveImagePath(meta.Image, path)
	if err != nil {
		return card{}, err
	}

	imageWebPath := ""
	thumbWebPath := ""
	if imagePath != "" {
		relImagePath, err := filepath.Rel(inputDir, imagePath)
		if err != nil {
			return card{}, err
		}
		if strings.HasPrefix(relImagePath, "..") {
			return card{}, fmt.Errorf("image must be inside input directory: %s", imagePath)
		}

		targetImagePath := filepath.Join(outputDir, "assets", relImagePath)
		if err := copyFile(imagePath, targetImagePath); err != nil {
			return card{}, fmt.Errorf("copy image %s: %w", imagePath, err)
		}
		imageWebPath = filepath.ToSlash(filepath.Join("assets", relImagePath))
		thumbWebPath = imageWebPath

		thumbRelPath, generated, err := maybeCreateThumbnail(imagePath, relImagePath, outputDir)
		if err != nil {
			return card{}, fmt.Errorf("create thumbnail %s: %w", imagePath, err)
		}
		if generated {
			thumbWebPath = filepath.ToSlash(filepath.Join("assets", thumbRelPath))
		}
	}

	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(relMarkdownPath), filepath.Ext(relMarkdownPath))
	}

	return card{
		ID:          fmt.Sprintf("card-%d", index),
		Title:       title,
		Author:      strings.TrimSpace(meta.Author),
		HTML:        template.HTML(htmlContent),
		PreviewHTML: template.HTML(htmlContent),
		Image:       imageWebPath,
		ThumbImage:  thumbWebPath,
	}, nil
}

func maybeCreateThumbnail(imagePath, relImagePath, outputDir string) (string, bool, error) {
	ext := strings.ToLower(filepath.Ext(imagePath))
	if ext != ".jpg" && ext != ".jpeg" {
		return "", false, nil
	}

	srcFile, err := os.Open(imagePath)
	if err != nil {
		return "", false, err
	}
	defer srcFile.Close()

	srcImage, _, err := image.Decode(srcFile)
	if err != nil {
		return "", false, err
	}

	bounds := srcImage.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	dstWidth, dstHeight := fitInside(srcWidth, srcHeight, thumbMaxWidth, thumbMaxHeight)
	if dstWidth == srcWidth && dstHeight == srcHeight {
		return "", false, nil
	}

	resized := resizeNearest(srcImage, dstWidth, dstHeight)
	thumbRelPath := filepath.Join("thumbs", strings.TrimSuffix(relImagePath, filepath.Ext(relImagePath))+".jpg")
	thumbOutputPath := filepath.Join(outputDir, "assets", thumbRelPath)
	if err := os.MkdirAll(filepath.Dir(thumbOutputPath), 0o755); err != nil {
		return "", false, err
	}

	dstFile, err := os.Create(thumbOutputPath)
	if err != nil {
		return "", false, err
	}
	defer dstFile.Close()

	if err := jpeg.Encode(dstFile, resized, &jpeg.Options{Quality: thumbJPEGQ}); err != nil {
		return "", false, err
	}

	if err := dstFile.Close(); err != nil {
		return "", false, err
	}

	return thumbRelPath, true, nil
}

func fitInside(srcWidth, srcHeight, maxWidth, maxHeight int) (int, int) {
	if srcWidth <= 0 || srcHeight <= 0 {
		return maxWidth, maxHeight
	}
	if srcWidth <= maxWidth && srcHeight <= maxHeight {
		return srcWidth, srcHeight
	}

	widthLimitedHeight := srcHeight * maxWidth / srcWidth
	if widthLimitedHeight <= maxHeight {
		return maxWidth, max(1, widthLimitedHeight)
	}

	heightLimitedWidth := srcWidth * maxHeight / srcHeight
	return max(1, heightLimitedWidth), maxHeight
}

func resizeNearest(src image.Image, dstWidth, dstHeight int) *image.RGBA {
	if dstWidth < 1 {
		dstWidth = 1
	}
	if dstHeight < 1 {
		dstHeight = 1
	}

	srcBounds := src.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	for y := 0; y < dstHeight; y++ {
		srcY := srcBounds.Min.Y + y*srcHeight/dstHeight
		for x := 0; x < dstWidth; x++ {
			srcX := srcBounds.Min.X + x*srcWidth/dstWidth
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	flattened := image.NewRGBA(dst.Bounds())
	draw.Draw(flattened, flattened.Bounds(), image.NewUniform(image.White), image.Point{}, draw.Src)
	draw.Draw(flattened, flattened.Bounds(), dst, image.Point{}, draw.Over)

	return flattened
}

func extractFrontMatter(raw []byte) (frontMatter, string, error) {
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return frontMatter{}, content, nil
	}

	end := strings.Index(content[4:], "\n---\n")
	if end == -1 {
		return frontMatter{}, "", errors.New("invalid front matter: missing closing '---'")
	}

	front := content[4 : 4+end]
	body := content[4+end+5:]

	var meta frontMatter
	if err := yaml.Unmarshal([]byte(front), &meta); err != nil {
		return frontMatter{}, "", fmt.Errorf("parse front matter yaml: %w", err)
	}

	return meta, body, nil
}

func markdownToHTML(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func resolveImagePath(frontMatterImage, markdownPath string) (string, error) {
	if strings.TrimSpace(frontMatterImage) != "" {
		candidate := filepath.Clean(filepath.Join(filepath.Dir(markdownPath), frontMatterImage))
		if _, err := os.Stat(candidate); err != nil {
			return "", fmt.Errorf("image from front matter not found: %s", candidate)
		}
		return candidate, nil
	}

	base := strings.TrimSuffix(markdownPath, filepath.Ext(markdownPath))
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"} {
		candidate := base + ext
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", nil
}

func copyFile(sourcePath, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}

func writeFavicon(outputDir string) error {
	targetPath := filepath.Join(outputDir, "assets", "logo.svg")
	if err := os.WriteFile(targetPath, defaultLogoSVG, 0o644); err != nil {
		return err
	}
	return nil
}

func loadConfig(path string) (appConfig, error) {
	config := defaultConfig()

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config, nil
		}
		return appConfig{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(raw, &config); err != nil {
		return appConfig{}, fmt.Errorf("parse config yaml %s: %w", path, err)
	}

	if strings.TrimSpace(config.Page.Title) == "" {
		config.Page.Title = defaultConfig().Page.Title
	}
	if strings.TrimSpace(config.Page.Subtitle) == "" {
		config.Page.Subtitle = defaultConfig().Page.Subtitle
	}

	return config, nil
}

func writeIndexHTML(path string, config appConfig, cards []card) error {
	const pageTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>{{ .PageTitle }}</title>
	<link rel="icon" type="image/svg+xml" href="assets/logo.svg" />
  <style>
    :root {
      --bg-a: #f4efe6;
      --bg-b: #e6efe8;
      --ink: #1d2a26;
      --card: #fffdf8;
      --line: #d4cec1;
      --accent: #bf4a35;
      --shadow: 0 16px 40px rgba(36, 30, 10, 0.12);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      color: var(--ink);
      font-family: "Avenir Next", "Segoe UI", sans-serif;
      background: linear-gradient(160deg, var(--bg-a) 0%, var(--bg-b) 100%);
      min-height: 100vh;
    }
    .wrap {
      max-width: 1100px;
      margin: 0 auto;
      padding: 3rem 1.2rem 4rem;
    }
    h1 {
      margin: 0 0 0.5rem;
      font-size: clamp(2rem, 2.7vw, 3rem);
      letter-spacing: 0.02em;
    }
    .lead {
			margin: 0;
      font-size: 1.05rem;
      max-width: 58ch;
      line-height: 1.55;
      opacity: 0.9;
    }
		.lead + .lead {
			margin-top: 0.4rem;
			margin-bottom: 2rem;
			font-size: 0.98rem;
			opacity: 0.8;
		}
    .board {
			display: flex;
			margin-top: 1rem;
			gap: 0.3rem;
			align-items: flex-start;
			opacity: 0;
			transition: opacity 120ms ease;
    }
		.image-loading {
			display: inline-flex;
			align-items: center;
			gap: 0.5rem;
			margin-top: 0.6rem;
			font-size: 0.92rem;
			letter-spacing: 0.01em;
			opacity: 0.9;
		}
		.image-loading[hidden] {
			display: none;
		}
		.image-loading::before {
			content: "";
			width: 0.9rem;
			height: 0.9rem;
			border: 2px solid var(--line);
			border-top-color: var(--accent);
			border-radius: 50%;
			animation: spin 0.9s linear infinite;
		}
    .column {
			flex: 1 1 0;
			display: flex;
			flex-direction: column;
			gap: 0.3rem;
    }
    .card-source {
			position: absolute;
			left: -99999px;
			top: 0;
			visibility: hidden;
    }
    .card {
			width: 100%;
			height: auto;
			align-self: stretch;
      text-align: left;
      border: 1px solid var(--line);
      border-radius: 18px;
      padding: 1rem;
			margin: 0;
      background: var(--card);
      box-shadow: var(--shadow);
      cursor: pointer;
      transition: transform 180ms ease, box-shadow 180ms ease;
    }
    .card:hover { transform: translateY(-4px) rotate(-0.5deg); }
    .author {
      margin: 0;
      font-size: 0.92rem;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--accent);
    }
    .title {
      margin: 0.35rem 0;
      font-size: 1.2rem;
    }
    .preview {
      margin: 0.5rem 0 0;
      line-height: 1.45;
      opacity: 0.9;
			text-wrap: pretty;
		}
		.preview p:first-child { margin-top: 0; }
		.preview p:last-child { margin-bottom: 0; }
		.preview ul, .preview ol {
			margin: 0.5rem 0;
			padding-left: 1.2rem;
    }
    .thumb {
      width: 100%;
      max-height: 180px;
      object-fit: cover;
      border-radius: 12px;
      margin-top: 0.8rem;
      border: 1px solid var(--line);
      background: #f3ede3;
    }
    .dialog {
      border: none;
      border-radius: 20px;
      padding: 0;
      width: min(880px, calc(100vw - 1.2rem));
      max-height: 90vh;
      box-shadow: 0 40px 80px rgba(0, 0, 0, 0.32);
    }
    .dialog::backdrop {
      background: rgba(16, 19, 20, 0.66);
      backdrop-filter: blur(4px);
    }
    .dialog-inner {
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: 20px;
      overflow: auto;
      max-height: 90vh;
      padding: 1.25rem 1.2rem 1.8rem;
    }
    .dialog-top {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 1rem;
      margin-bottom: 0.9rem;
    }
    .close {
      border: 1px solid var(--line);
      background: #f8f3e9;
      color: var(--ink);
      border-radius: 999px;
      width: 34px;
      height: 34px;
      cursor: pointer;
      font-size: 1.1rem;
      line-height: 1;
    }
    .content {
      line-height: 1.6;
      font-size: 1.03rem;
    }
    .content p:first-child { margin-top: 0; }
    .hero {
      width: 100%;
      max-height: 380px;
      object-fit: cover;
      border-radius: 14px;
      border: 1px solid var(--line);
      margin-bottom: 1rem;
    }
		.site-footer {
			margin-top: 1.5rem;
			font-size: 0.68rem;
			opacity: 0.65;
			letter-spacing: 0.02em;
			text-align: center;
		}
		.site-footer p {
			margin: 0;
		}
		.site-footer a {
			color: inherit;
		}
		.site-footer a:hover {
			opacity: 0.8;
		}
		@keyframes spin {
			to { transform: rotate(360deg); }
		}
		@media (max-width: 640px) {
			.column {
				width: 100%;
			}
      .wrap { padding-top: 2rem; }
      .dialog-inner { padding: 1rem; }
    }
  </style>
</head>
<body>
  <main class="wrap">
		<h1>{{ .PageTitle }}</h1>
		<p class="lead">{{ .PageSubtitle }}</p>
		<p id="image-loading" class="image-loading" aria-live="polite" hidden>Loading photos...</p>
		<section class="board" aria-label="Patchwork board">
			<div class="column" data-column></div>
			<div class="column" data-column></div>
			<div class="column" data-column></div>
		</section>
		<div id="card-source" class="card-source">
			{{- range .Cards }}
			<button type="button" class="card" data-target="{{ .ID }}">
				<p class="author">{{ .Author }}</p>
				<h2 class="title">{{ .Title }}</h2>
		<section class="preview">{{ .PreviewHTML }}</section>
				{{- if .Image }}
				<img src="{{ .ThumbImage }}" alt="Image for {{ .Title }}" class="thumb" loading="eager" decoding="async" fetchpriority="low" data-track-load="true" />
				{{- end }}
			</button>
			{{- end }}
		</div>
		{{- if .FooterEnabled }}
		<footer class="site-footer">
			<p>built with <span aria-hidden="true">&#10084;&#65039;</span> with <a href="https://github.com/aae42/patchwork">patchwork</a></p>
		</footer>
		{{- end }}
  </main>

  {{- range .Cards }}
  <dialog id="{{ .ID }}" class="dialog">
    <article class="dialog-inner">
      <div class="dialog-top">
        <div>
          <p class="author">{{ .Author }}</p>
          <h2 class="title">{{ .Title }}</h2>
        </div>
        <button class="close" type="button" aria-label="Close">X</button>
      </div>
      {{- if .Image }}
			<img data-src="{{ .Image }}" alt="Image for {{ .Title }}" class="hero lazy-hero" loading="lazy" decoding="async" />
      {{- end }}
      <section class="content">{{ .HTML }}</section>
    </article>
  </dialog>
  {{- end }}

  <script>
		function bindDialogs() {
			for (const dialog of document.querySelectorAll('dialog')) {
				const closeBtn = dialog.querySelector('.close');
				if (closeBtn) {
					closeBtn.addEventListener('click', () => dialog.close());
				}

				dialog.addEventListener('click', (event) => {
					const rect = dialog.getBoundingClientRect();
					const inDialog = rect.top <= event.clientY && event.clientY <= rect.bottom &&
						rect.left <= event.clientX && event.clientX <= rect.right;
					if (!inDialog) dialog.close();
				});
				dialog.addEventListener('close', () => {
					document.body.style.overflow = '';
				});
			}
		}

		function bindCardClicks() {
			for (const card of document.querySelectorAll('[data-target]')) {
				card.addEventListener('click', () => {
					const dialog = document.getElementById(card.dataset.target);
					if (dialog) {
						hydrateDialogImage(dialog);
						dialog.showModal();
						document.body.style.overflow = 'hidden';
					}
				});
			}
		}

		function hydrateDialogImage(dialog) {
			const img = dialog.querySelector('.lazy-hero[data-src]');
			if (img && !img.getAttribute('src')) {
				img.setAttribute('src', img.dataset.src);
			}
		}

		function bindImageLoadingIndicator() {
			const indicator = document.getElementById('image-loading');
			const thumbs = [...document.querySelectorAll('.thumb[data-track-load="true"]')];
			if (!indicator || thumbs.length === 0) {
				return;
			}

			let pending = 0;
			const onDone = () => {
				pending -= 1;
				if (pending <= 0) {
					indicator.hidden = true;
				}
			};

			for (const image of thumbs) {
				if (image.complete) {
					continue;
				}
				pending += 1;
				image.addEventListener('load', onDone, { once: true });
				image.addEventListener('error', onDone, { once: true });
			}

			indicator.hidden = pending === 0;
			if (pending > 0) {
				window.setTimeout(() => {
					indicator.hidden = true;
				}, 12000);
			}
		}

		function layoutBoard() {
			const board = document.querySelector('.board');
			const columns = [...document.querySelectorAll('[data-column]')];
			const source = document.getElementById('card-source');
			const desiredColumns = window.innerWidth <= 640 ? 1 : window.innerWidth <= 980 ? 2 : 3;
			const cards = [...document.querySelectorAll('#card-source .card, .board .card')];

			columns.forEach((column, index) => {
				column.style.display = index < desiredColumns ? 'flex' : 'none';
			});

			const visibleColumns = columns.slice(0, desiredColumns);

			for (const column of visibleColumns) {
				column.replaceChildren();
			}
			for (const card of cards) {
				source.appendChild(card);
			}

			const measureHost = document.createElement('div');
			measureHost.style.position = 'absolute';
			measureHost.style.left = '-99999px';
			measureHost.style.top = '0';
			measureHost.style.visibility = 'hidden';
			measureHost.style.width = visibleColumns[0].getBoundingClientRect().width + 'px';
			document.body.appendChild(measureHost);

			for (const card of cards) {
				const probe = card.cloneNode(true);
				probe.style.width = '100%';
				measureHost.appendChild(probe);
				const shortestColumn = visibleColumns.reduce((shortest, candidate) => {
					return candidate.offsetHeight < shortest.offsetHeight ? candidate : shortest;
				});
				shortestColumn.appendChild(card);
				measureHost.removeChild(probe);
			}

			document.body.removeChild(measureHost);
			source.style.display = 'none';
			board.style.opacity = '1';
		}

		function bindThumbRelayout() {
			for (const image of document.querySelectorAll('#card-source .thumb')) {
				image.addEventListener('load', debouncedLayoutBoard, { once: true });
				image.addEventListener('error', debouncedLayoutBoard, { once: true });
			}
		}

		let layoutTimer;
		function debouncedLayoutBoard() {
			window.clearTimeout(layoutTimer);
			layoutTimer = window.setTimeout(layoutBoard, 90);
		}

		bindDialogs();
		bindCardClicks();
		bindThumbRelayout();
		bindImageLoadingIndicator();

		window.addEventListener('DOMContentLoaded', () => {
			layoutBoard();
		});

		let resizeTimer;
		let lastWidth = window.innerWidth;
		window.addEventListener('resize', () => {
			const currentWidth = window.innerWidth;
			if (currentWidth === lastWidth) return;
			lastWidth = currentWidth;
			window.clearTimeout(resizeTimer);
			resizeTimer = window.setTimeout(layoutBoard, 120);
		});
  </script>
</body>
</html>
`

	tmpl, err := template.New("index").Parse(pageTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "<!-- built with patchwork v%s -->\n", version)

	if err := tmpl.Execute(f, pageData{
		PageTitle:     config.Page.Title,
		PageSubtitle:  config.Page.Subtitle,
		FooterEnabled: config.Footer.Enabled,
		Cards:         cards,
	}); err != nil {
		return err
	}

	return f.Close()
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

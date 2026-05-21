package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

type frontMatter struct {
	Author string `yaml:"author"`
	Image  string `yaml:"image"`
	Title  string `yaml:"title"`
}

type card struct {
	ID      string
	Title   string
	Author  string
	HTML    template.HTML
	Preview string
	Image   string
}

type pageData struct {
	PageTitle    string
	PageSubtitle string
	Cards        []card
}

type appConfig struct {
	Page pageConfig `yaml:"page"`
}

type pageConfig struct {
	Title    string `yaml:"title"`
	Subtitle string `yaml:"subtitle"`
}

func defaultConfig() appConfig {
	return appConfig{
		Page: pageConfig{
			Title:    "Patchwork Cards",
			Subtitle: "Each tile is a stitched-together note from the people who care about you most.",
		},
	}
}

var whitespaceRE = regexp.MustCompile(`\s+`)

func main() {
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
	}

	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(relMarkdownPath), filepath.Ext(relMarkdownPath))
	}

	return card{
		ID:      fmt.Sprintf("card-%d", index),
		Title:   title,
		Author:  strings.TrimSpace(meta.Author),
		HTML:    template.HTML(htmlContent),
		Preview: buildPreview(body),
		Image:   imageWebPath,
	}, nil
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

func buildPreview(markdown string) string {
	clean := strings.TrimSpace(markdown)
	clean = strings.ReplaceAll(clean, "#", "")
	clean = strings.ReplaceAll(clean, "*", "")
	clean = strings.ReplaceAll(clean, "_", "")
	clean = strings.ReplaceAll(clean, "`", "")
	clean = whitespaceRE.ReplaceAllString(clean, " ")

	if len(clean) <= 160 {
		return clean
	}
	return strings.TrimSpace(clean[:157]) + "..."
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
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
      gap: 1rem;
    }
    .card {
      width: 100%;
      text-align: left;
      border: 1px solid var(--line);
      border-radius: 18px;
      padding: 1rem;
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
    @media (max-width: 640px) {
      .wrap { padding-top: 2rem; }
      .dialog-inner { padding: 1rem; }
    }
  </style>
</head>
<body>
  <main class="wrap">
		<h1>{{ .PageTitle }}</h1>
		<p class="lead">{{ .PageSubtitle }}</p>
		<p class="lead">Click any card to open the full note.</p>
    <section class="grid">
      {{- range .Cards }}
      <button type="button" class="card" data-target="{{ .ID }}">
        <p class="author">{{ .Author }}</p>
        <h2 class="title">{{ .Title }}</h2>
        <p class="preview">{{ .Preview }}</p>
        {{- if .Image }}
        <img src="{{ .Image }}" alt="Image for {{ .Title }}" class="thumb" />
        {{- end }}
      </button>
      {{- end }}
    </section>
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
      <img src="{{ .Image }}" alt="Image for {{ .Title }}" class="hero" />
      {{- end }}
      <section class="content">{{ .HTML }}</section>
    </article>
  </dialog>
  {{- end }}

  <script>
    for (const card of document.querySelectorAll('[data-target]')) {
      card.addEventListener('click', () => {
        const dialog = document.getElementById(card.dataset.target);
        if (dialog) dialog.showModal();
      });
    }

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
    }
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

	if err := tmpl.Execute(f, pageData{
		PageTitle:    config.Page.Title,
		PageSubtitle: config.Page.Subtitle,
		Cards:        cards,
	}); err != nil {
		return err
	}

	return f.Close()
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

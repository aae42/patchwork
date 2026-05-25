# this list
default:
  @just --list --unsorted

version := trim(`cat VERSION`)

# build the binary
build:
  mkdir -p build
  go build -ldflags "-s -w -X main.version={{version}}" -o build/patchwork main.go

# build the nix flake
build-nix:
  nix build

# build everything
build-all:
  just build
  just build-nix

# run patchwork against the example board
run INPUT='examples/boards/celebration':
  go run main.go {{INPUT}} public/

# serve the example board locally on port 8080, and watch for changes to rebuild and refresh
serve:
  watchexec --restart -w "./" \
    'just build; caddy file-server --listen :8080 --root public/'

# release a new version on github by tagging the current version
[confirm]
release:
  git tag "v$(cat VERSION)"
  git push origin "v$(cat VERSION)"

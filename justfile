# this list
default:
  @just --list --unsorted

version := trim(`cat VERSION`)

build:
  mkdir -p build
  go build -ldflags "-s -w -X main.version={{version}}" -o build/patchwork main.go

build-nix:
  nix build

build-all:
  just build
  just build-nix

run INPUT='examples/boards/celebration':
  go run main.go {{INPUT}} public/

serve:
  caddy file-server --browse --listen :8080 --root public/

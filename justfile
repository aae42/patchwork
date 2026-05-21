# this list
default:
  @just --list --unsorted

build:
  mkdir -p build
  go build -o build/patchwork main.go

run INPUT='examples/boards/celebration':
  go run main.go {{INPUT}} public/

serve:
  caddy file-server --browse --listen :8080 --root public/

#!/bin/sh
# denma: installs the Go and Bun that builds use (listmonk-infra's CodeBuild
# project), each checked against its checksum. A new .go-version needs its
# version and checksums here too (go.dev/dl/?mode=json).
#   .github/denma-tools.sh <dir>   then PATH=<dir>/go/bin:<dir>/bin:$PATH
set -eu

GO_VERSION=1.26.11
GO_SHA256_arm64=6c2f29b458b5add0835f18b776f69c8881a6d88428f1484a6da55755e620757e
GO_SHA256_amd64=9849f4e04ee35303714e34fbc3ac8ab44ea3e448a95aa182f91e3b9b79c710ec
BUN_VERSION=1.4.4
BUN_SHA256_arm64=baa6bb36f51d6e58561e02bbea825038a9df454f673d6d776e8bbf86f8c7850e
BUN_SHA256_amd64=bf8ccbfb2b0b80159e0f9a6d15c55317add8077534dbc6130a3edb52fccb395c

DIR=$1
case "$(uname -m)" in
aarch64 | arm64) ARCH=arm64 BUN_ARCH=aarch64 go_sum=$GO_SHA256_arm64 bun_sum=$BUN_SHA256_arm64 ;;
x86_64 | amd64) ARCH=amd64 BUN_ARCH=x64 go_sum=$GO_SHA256_amd64 bun_sum=$BUN_SHA256_amd64 ;;
*) echo "No tools for $(uname -m)" >&2; exit 1 ;;
esac
if [ "$(cat "$(dirname "$0")/../.go-version")" != "$GO_VERSION" ]; then
    echo ".go-version isn't $GO_VERSION: update the version and checksums in $0" >&2
    exit 1
fi

# fetch URL SHA256 FILE
fetch() {
    curl -fsSL --retry 3 -o "$3" "$1"
    echo "$2  $3" | sha256sum -c --quiet -
}

mkdir -p "$DIR/bin"
fetch "https://go.dev/dl/go$GO_VERSION.linux-$ARCH.tar.gz" "$go_sum" "$DIR/go.tgz"
rm -rf "$DIR/go"
tar -xzf "$DIR/go.tgz" -C "$DIR"
fetch "https://github.com/oven-sh/bun/releases/download/bun-v$BUN_VERSION/bun-linux-$BUN_ARCH.zip" "$bun_sum" "$DIR/bun.zip"
unzip -q -o -j "$DIR/bun.zip" "bun-linux-$BUN_ARCH/bun" -d "$DIR/bin"
rm -f "$DIR/go.tgz" "$DIR/bun.zip"
echo "Go $("$DIR/go/bin/go" env GOVERSION), Bun $("$DIR/bin/bun" --version), linux/$ARCH"

#!/usr/bin/env bash
set -euo pipefail

# Usage: curl -fsSL https://raw.githubusercontent.com/axo-clash/cli/main/install.sh | bash
main() (
  local repo='https://github.com/axo-clash/cli' version="${AXO_CLASH_VERSION:-latest}"
  local install_dir="${INSTALL_DIR:-$HOME/.local/bin}" os arch asset tmp expected actual resolved
  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo 'Système non supporté : utilise install.ps1 pour Windows.' >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo 'Architecture non supportée (x64 et ARM64 uniquement).' >&2; exit 1 ;;
  esac
  command -v curl >/dev/null || { echo 'curl est requis.' >&2; exit 1; }
  if [[ "$version" == latest ]]; then
    resolved=$(curl --proto '=https' --tlsv1.2 -fsSL -o /dev/null -w '%{url_effective}' "$repo/releases/latest")
    version="${resolved##*/}"
  fi
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo 'Version invalide (ex. v1.2.3).' >&2; exit 1; }
  asset="axo-clash_${os}_${arch}"
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  curl --proto '=https' --tlsv1.2 -fsSL "$repo/releases/download/$version/$asset" -o "$tmp/$asset"
  curl --proto '=https' --tlsv1.2 -fsSL "$repo/releases/download/$version/checksums.txt" -o "$tmp/checksums.txt"
  expected=$(awk -v name="$asset" '$2 == name {print $1}' "$tmp/checksums.txt")
  [[ "$expected" =~ ^[0-9a-f]{64}$ ]] || { echo 'Somme SHA-256 absente ou invalide.' >&2; exit 1; }
  if command -v sha256sum >/dev/null; then
    actual=$(sha256sum "$tmp/$asset"); actual="${actual%% *}"
  elif command -v shasum >/dev/null; then
    actual=$(shasum -a 256 "$tmp/$asset"); actual="${actual%% *}"
  else
    echo 'sha256sum ou shasum est requis.' >&2; exit 1
  fi
  [[ "$actual" == "$expected" ]] || { echo 'Échec de la vérification SHA-256.' >&2; exit 1; }
  mkdir -p "$install_dir"
  install -m 755 "$tmp/$asset" "$install_dir/axo-clash"
  printf 'AXO Clash %s installé dans %s/axo-clash\n' "$version" "$install_dir"
  # Keep $PATH literal in the suggested shell configuration.
  # shellcheck disable=SC2016
  case ":$PATH:" in
    *":$install_dir:"*) ;;
    *) printf 'Ajoute ce dossier au PATH (dans ~/.zshrc ou ~/.bashrc) :\nexport PATH="%s:$PATH"\n' "$install_dir" ;;
  esac
)
main "$@"

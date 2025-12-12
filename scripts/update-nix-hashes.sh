#!/usr/bin/env bash
# shellcheck disable=SC2034  # Namerefs appear unused to shellcheck
set -euo pipefail

readonly GITHUB_REPO="${GITHUB_REPO:-smykla-labs/klaudiush}"
readonly RELEASE_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download"

main() {
  local version="${1:-}"

  validate_inputs "${version}"

  local script_dir project_root package_file
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  project_root="${script_dir}/.."
  package_file="${project_root}/nix/package.nix"

  version="${version#v}"

  echo "Updating Nix package hashes for version ${version}"

  local -A platform_hashes
  fetch_and_hash_artifacts "${version}" platform_hashes
  update_package_file "${version}" "${package_file}" platform_hashes

  echo "Successfully updated ${package_file}"
}

validate_inputs() {
  local version="${1}"

  if [[ -z "${version}" ]]; then
    echo "Usage: ${0} <version>" >&2
    echo "Example: ${0} 1.12.5" >&2
    return 1
  fi
}

fetch_and_hash_artifacts() {
  local version="${1}"
  local -n result_hashes="${2}"

  local -Ar platforms=(
    ["darwin_arm64"]=aarch64-darwin
    ["darwin_amd64"]=x86_64-darwin
    ["linux_amd64"]=x86_64-linux
    ["linux_arm64"]=aarch64-linux
  )

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' RETURN

  echo "Fetching and hashing release artifacts..."

  local goreleaser_name nix_platform tarball url base32_hash sri_hash
  for goreleaser_name in "${!platforms[@]}"; do
    nix_platform="${platforms["${goreleaser_name}"]}"
    tarball="klaudiush_${version}_${goreleaser_name}.tar.gz"
    url="${RELEASE_BASE_URL}/v${version}/${tarball}"

    echo "  ${nix_platform}"

    download_artifact "${url}" "${tmp_dir}/${tarball}"

    base32_hash="$(nix-prefetch-url --type sha256 "file://${tmp_dir}/${tarball}" 2>/dev/null)"
    sri_hash="$(nix hash convert --hash-algo sha256 --to sri "${base32_hash}")"

    # shellcheck disable=SC2004  # Platform name is a string, not arithmetic
    result_hashes[${nix_platform}]="${sri_hash}"
  done
}

download_artifact() {
  local url="${1}"
  local output_path="${2}"

  if ! curl --fail --silent --location "${url}" --output "${output_path}"; then
    echo "Error: Failed to download ${url}" >&2
    return 1
  fi
}

update_package_file() {
  local version="${1}"
  local pkg_file="${2}"
  local -n source_hashes="${3}"

  if [[ ! -f "${pkg_file}" ]]; then
    echo "Error: ${pkg_file} not found" >&2
    return 1
  fi

  local output_file
  output_file="$(mktemp)"
  trap 'rm -f "${output_file}"' RETURN

  awk \
    -v version="${version}" \
    -v hash_aarch64_darwin="${source_hashes["aarch64-darwin"]}" \
    -v hash_x86_64_darwin="${source_hashes["x86_64-darwin"]}" \
    -v hash_x86_64_linux="${source_hashes["x86_64-linux"]}" \
    -v hash_aarch64_linux="${source_hashes["aarch64-linux"]}" \
    '
    /^  version = / {
      print "  version = \"" version "\";"
      next
    }
    /aarch64-darwin/ && /hash = / {
      sub(/hash = "[^"]*"/, "hash = \"" hash_aarch64_darwin "\"")
    }
    /x86_64-darwin/ && /hash = / {
      sub(/hash = "[^"]*"/, "hash = \"" hash_x86_64_darwin "\"")
    }
    /x86_64-linux/ && /hash = / {
      sub(/hash = "[^"]*"/, "hash = \"" hash_x86_64_linux "\"")
    }
    /aarch64-linux/ && /hash = / {
      sub(/hash = "[^"]*"/, "hash = \"" hash_aarch64_linux "\"")
    }
    { print }
    ' "${pkg_file}" > "${output_file}"

  mv "${output_file}" "${pkg_file}"

  print_summary "${version}" source_hashes
}

print_summary() {
  local version="${1}"
  local -n hashes="${2}"

  echo ""
  echo "Updated hashes:"
  echo "  Version:        ${version}"
  echo "  aarch64-darwin: ${hashes["aarch64-darwin"]}"
  echo "  x86_64-darwin:  ${hashes["x86_64-darwin"]}"
  echo "  x86_64-linux:   ${hashes["x86_64-linux"]}"
  echo "  aarch64-linux:  ${hashes["aarch64-linux"]}"
}

main "$@"

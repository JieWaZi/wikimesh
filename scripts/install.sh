#!/usr/bin/env sh
set -eu

OWNER="${WIKIMESH_OWNER:-JieWaZi}"
REPO="${WIKIMESH_REPO:-wikimesh}"
BINARY_NAME="${WIKIMESH_BINARY:-wikimesh}"
VERSION="${VERSION:-}"
INSTALL_DIR="${WIKIMESH_INSTALL_DIR:-${INSTALL_DIR:-}}"

log() {
	printf '%s\n' "$*" >&2
}

fail() {
	log "error: $*"
	exit 1
}

download_file() {
	url="$1"
	output="$2"

	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$output"
		return
	fi
	if command -v wget >/dev/null 2>&1; then
		wget -qO "$output" "$url"
		return
	fi

	fail "curl or wget is required"
}

detect_os() {
	case "$(uname -s)" in
		Darwin) printf 'darwin' ;;
		Linux) printf 'linux' ;;
		*) fail "unsupported OS: $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
		x86_64|amd64) printf 'amd64' ;;
		arm64|aarch64) printf 'arm64' ;;
		*) fail "unsupported architecture: $(uname -m)" ;;
	esac
}

asset_name() {
	os="$1"
	arch="$2"

	case "${os}-${arch}" in
		linux-amd64|linux-arm64) printf '%s-%s-%s.tar.gz' "$BINARY_NAME" "$os" "$arch" ;;
		darwin-amd64) printf '%s-darwin-amd64.tar.gz' "$BINARY_NAME" ;;
		darwin-arm64) printf '%s-darwin-arm64.tar.gz' "$BINARY_NAME" ;;
		*) fail "unsupported release platform: ${os}/${arch}" ;;
	esac
}

binary_name_for_asset() {
	asset="$1"
	name="${asset%.tar.gz}"
	printf '%s' "$name"
}

sha256_file() {
	file="$1"

	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | awk '{print $1}'
		return
	fi
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | awk '{print $1}'
		return
	fi
	if command -v openssl >/dev/null 2>&1; then
		openssl dgst -sha256 "$file" | awk '{print $NF}'
		return
	fi

	fail "missing sha256sum, shasum, or openssl"
}

resolve_install_dir() {
	if [ -n "$INSTALL_DIR" ]; then
		printf '%s' "$INSTALL_DIR"
		return
	fi

	printf '/usr/local/bin'
}

checksum_for_asset() {
	checksums_path="$1"
	asset="$2"

	hash="$(awk -v asset="./${asset}" '$2 == asset {print $1}' "$checksums_path")"
	if [ -z "$hash" ]; then
		hash="$(awk -v asset="${asset}" '$2 == asset {print $1}' "$checksums_path")"
	fi
	[ -n "$hash" ] || fail "unable to find checksum for ${asset}"
	printf '%s' "$hash"
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

os="$(detect_os)"
arch="$(detect_arch)"
asset="$(asset_name "$os" "$arch")"
archive_binary="$(binary_name_for_asset "$asset")"
install_dir="$(resolve_install_dir)"
if [ -n "$VERSION" ]; then
	base_url="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}"
else
	base_url="https://github.com/${OWNER}/${REPO}/releases/latest/download"
fi
checksums_path="${tmp_dir}/checksums.txt"
archive_path="${tmp_dir}/${asset}"
binary_path="${tmp_dir}/${archive_binary}"
use_sudo=0

download_file "${base_url}/checksums.txt" "${checksums_path}"

log "downloading ${asset}"
download_file "${base_url}/${asset}" "${archive_path}"

expected_hash="$(checksum_for_asset "${checksums_path}" "${asset}")"
actual_hash="$(sha256_file "${archive_path}")"
[ "$expected_hash" = "$actual_hash" ] || fail "checksum verification failed for ${asset}"

tar -xzf "${archive_path}" -C "${tmp_dir}"
[ -f "${binary_path}" ] || fail "archive did not contain ${archive_binary}"

if [ ! -w "${install_dir}" ]; then
	if [ "${install_dir}" = "/usr/local/bin" ] && command -v sudo >/dev/null 2>&1; then
		use_sudo=1
	else
		install_dir="${HOME}/.local/bin"
	fi
fi

if [ "$use_sudo" -eq 1 ]; then
	sudo mkdir -p "${install_dir}"
	sudo install -m 0755 "${binary_path}" "${install_dir}/${BINARY_NAME}"
else
	mkdir -p "${install_dir}"
	install -m 0755 "${binary_path}" "${install_dir}/${BINARY_NAME}"
fi

log "installed ${BINARY_NAME} to ${install_dir}/${BINARY_NAME}"
case ":$PATH:" in
	*:"${install_dir}":*)
		log "run: ${BINARY_NAME} --help"
		;;
	*)
		log "add ${install_dir} to PATH, then run: ${BINARY_NAME} --help"
		;;
esac

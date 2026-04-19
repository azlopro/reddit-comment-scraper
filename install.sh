#!/bin/bash
# install.sh — one-shot installer for reddit-monitor
# Usage: curl -fsSL https://raw.githubusercontent.com/azlopro/reddit-monitor/main/install.sh | sudo bash
set -euo pipefail

REPO="azlopro/reddit-comment-scraper"
BIN_NAME="reddit-monitor"
INSTALL_DIR="/opt/reddit-monitor"
CONFIG_DIR="/etc/reddit-monitor"
SERVICE_USER="reddit-monitor"
SERVICE_FILE="/usr/lib/systemd/system/reddit-monitor.service"
NOLOGIN=$(command -v nologin 2>/dev/null || echo /usr/sbin/nologin)

# ── helpers ──────────────────────────────────────────────────────────────────

die()  { echo "ERROR: $*" >&2; exit 1; }
info() { echo "  → $*"; }

require_root() {
  [ "$(id -u)" -eq 0 ] || die "Run this script as root (sudo bash install.sh)"
}

detect_arch() {
  case "$(uname -m)" in
    x86_64)  echo amd64 ;;
    aarch64) echo arm64 ;;
    *)       die "Unsupported architecture: $(uname -m)" ;;
  esac
}

detect_pkg_manager() {
  for pm in apt dnf yum zypper pacman; do
    command -v "$pm" &>/dev/null && echo "$pm" && return
  done
  echo "none"
}

latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | cut -d '"' -f 4
}

release_url() {
  local asset="$1"
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep "browser_download_url" | grep "/${asset}\"" | cut -d '"' -f 4
}

download_and_verify() {
  local asset="$1" dest="$2"
  local url
  url=$(release_url "${asset}") || die "Could not find release asset: ${asset}"
  info "Downloading ${asset}"
  curl -fsSL "${url}" -o "${dest}"
  local checksum_url
  checksum_url=$(release_url "checksums.txt") || die "Could not find checksums.txt"
  curl -fsSL "${checksum_url}" -o /tmp/reddit-monitor-checksums.txt
  info "Verifying checksum"
  grep " ${asset}$" /tmp/reddit-monitor-checksums.txt | sha256sum --check --status \
    || die "Checksum verification failed for ${asset}"
  rm -f /tmp/reddit-monitor-checksums.txt
}

# ── user + directory setup ────────────────────────────────────────────────────

setup_user() {
  if ! id "${SERVICE_USER}" &>/dev/null; then
    info "Creating system user ${SERVICE_USER}"
    useradd --system --no-create-home --shell "${NOLOGIN}" \
      --comment "reddit-monitor daemon" "${SERVICE_USER}"
  fi
}

setup_dirs() {
  install -d -m 755 "${INSTALL_DIR}"
  install -d -m 755 "${CONFIG_DIR}"
}

# ── config ────────────────────────────────────────────────────────────────────

configure() {
  local env_file="${CONFIG_DIR}/env"
  if [ -f "${env_file}" ] && grep -q "DISCORD_WEBHOOK_URL=." "${env_file}"; then
    info "Config already set — skipping webhook prompt"
    return
  fi

  echo
  echo "Enter your Discord incoming webhook URL."
  echo "Create one: Server Settings → Integrations → Webhooks → New Webhook"
  read -r -p "  DISCORD_WEBHOOK_URL: " webhook_url
  [ -n "${webhook_url}" ] || die "Webhook URL cannot be empty"

  printf '# reddit-monitor configuration\nDISCORD_WEBHOOK_URL=%s\n' "${webhook_url}" \
    > "${env_file}"
  chown root:root "${env_file}"
  chmod 600       "${env_file}"
  info "Config written to ${env_file}"
}

# ── binary install ────────────────────────────────────────────────────────────

install_binary() {
  local arch="$1" version="$2"
  local asset="${BIN_NAME}-${arch}"
  download_and_verify "${asset}" "/tmp/${asset}"
  install -m 755 -o root -g root "/tmp/${asset}" "${INSTALL_DIR}/${BIN_NAME}"
  rm -f "/tmp/${asset}"
  info "Binary installed to ${INSTALL_DIR}/${BIN_NAME}"
}

install_package() {
  local pm="$1" arch="$2" version="$3"
  local pkg_ext
  case "${pm}" in
    apt)            pkg_ext="deb" ;;
    dnf|yum)        pkg_ext="rpm" ;;
    zypper)         pkg_ext="rpm" ;;
    pacman)         pkg_ext="pkg.tar.zst" ;;
    *)              return 1 ;;
  esac

  local asset="${BIN_NAME}_${version}_linux_${arch}.${pkg_ext}"
  download_and_verify "${asset}" "/tmp/${asset}" || return 1

  case "${pm}" in
    apt)     dpkg -i "/tmp/${asset}" ;;
    dnf|yum) "${pm}" localinstall -y "/tmp/${asset}" ;;
    zypper)  zypper install -y "/tmp/${asset}" ;;
    pacman)  pacman -U --noconfirm "/tmp/${asset}" ;;
  esac
  rm -f "/tmp/${asset}"
}

# ── systemd ───────────────────────────────────────────────────────────────────

install_service() {
  local version="$1"
  local asset="${BIN_NAME}.service"
  local url
  url=$(release_url "${asset}") || die "Could not find ${asset} in release"
  info "Installing systemd service"
  curl -fsSL "${url}" -o "${SERVICE_FILE}"
  chmod 644 "${SERVICE_FILE}"
}

enable_service() {
  systemctl daemon-reload
  if systemctl is-active --quiet "${BIN_NAME}"; then
    info "Restarting ${BIN_NAME}"
    systemctl restart "${BIN_NAME}"
  else
    info "Enabling and starting ${BIN_NAME}"
    systemctl enable --now "${BIN_NAME}"
  fi
}

# ── main ──────────────────────────────────────────────────────────────────────

main() {
  require_root
  local arch pm version
  arch=$(detect_arch)
  pm=$(detect_pkg_manager)
  version=$(latest_version) || die "Could not fetch latest release version"

  echo "Installing reddit-monitor ${version} (${arch})"

  setup_user
  setup_dirs

  install_binary "${arch}" "${version}"
  install_service "${version}"

  configure
  enable_service

  echo
  echo "Done. Check status with: journalctl -u reddit-monitor -f"
}

main "$@"

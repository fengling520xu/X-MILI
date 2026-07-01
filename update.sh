#!/usr/bin/env bash
set -euo pipefail

APP_NAME="X-MILI"
REPO="https://github.com/Aimilibot/X-MILI"
API_REPO="https://api.github.com/repos/Aimilibot/X-MILI"
INSTALL_DIR="${XUI_MAIN_FOLDER:-/usr/local/x-ui}"
COMMIT_FILE="${INSTALL_DIR}/.x-mili-commit"

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

log() { echo -e "${green}[${APP_NAME}]${plain} $*"; }
warn() { echo -e "${yellow}[${APP_NAME}]${plain} $*"; }
fail() { echo -e "${red}[${APP_NAME}]${plain} $*" >&2; exit 1; }

[[ $EUID -ne 0 ]] && fail "请使用 root 运行 / Please run as root"
command -v systemctl >/dev/null 2>&1 || fail "需要 systemd / systemd is required"
[[ -d "$INSTALL_DIR" ]] || fail "未找到安装目录 ${INSTALL_DIR}，请先安装"

GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
if [[ ! -x "$GO_BIN" ]]; then
    GO_BIN="$(command -v go || true)"
fi
[[ -n "$GO_BIN" && -x "$GO_BIN" ]] || fail "缺少 Go 编译环境，请先运行安装脚本"
command -v curl >/dev/null 2>&1 || fail "缺少 curl"
command -v tar >/dev/null 2>&1 || fail "缺少 tar"
command -v gcc >/dev/null 2>&1 || warn "未检测到 gcc；如果编译失败，请先安装 gcc"

remote_commit="$(curl -fsSL "${API_REPO}/git/ref/heads/main" | sed -n 's/.*"sha": "\([0-9a-f]\{40\}\)".*/\1/p' || true)"
local_commit="$(cat "$COMMIT_FILE" 2>/dev/null || true)"

if [[ -n "$remote_commit" && "$remote_commit" == "$local_commit" && "${X_MILI_FORCE_UPDATE:-0}" != "1" ]]; then
    log "已是最新版本，无需更新。"
    exit 0
fi

tmp_dir="$(mktemp -d -t x-mili-update.XXXXXX)"
backup_dir="$(mktemp -d -t x-mili-update-backup.XXXXXX)"
trap 'rm -rf "$tmp_dir" "$backup_dir"' EXIT

archive_ref="${remote_commit:-main}"
log "下载最新源码..."
curl -fL "${REPO}/archive/${archive_ref}.tar.gz" -o "$tmp_dir/source.tar.gz"
mkdir -p "$tmp_dir/src"
tar -xzf "$tmp_dir/source.tar.gz" -C "$tmp_dir/src" --strip-components=1

log "编译面板程序..."
cd "$tmp_dir/src"
mkdir -p build
"$GO_BIN" build -ldflags "-w -s" -o build/x-ui main.go

log "备份当前程序..."
[[ -f "$INSTALL_DIR/x-ui" ]] && cp -a "$INSTALL_DIR/x-ui" "$backup_dir/x-ui"
[[ -f /usr/bin/ml ]] && cp -a /usr/bin/ml "$backup_dir/ml"
[[ -f "$COMMIT_FILE" ]] && cp -a "$COMMIT_FILE" "$backup_dir/commit"

rollback() {
    warn "更新失败，正在回滚..."
    [[ -f "$backup_dir/x-ui" ]] && install -m 755 "$backup_dir/x-ui" "$INSTALL_DIR/x-ui"
    [[ -f "$backup_dir/ml" ]] && install -m 755 "$backup_dir/ml" /usr/bin/ml
    if [[ -f "$backup_dir/commit" ]]; then
        install -m 644 "$backup_dir/commit" "$COMMIT_FILE"
    else
        rm -f "$COMMIT_FILE"
    fi
    systemctl restart x-ui >/dev/null 2>&1 || true
}

log "替换程序文件和菜单..."
install -m 755 build/x-ui "$INSTALL_DIR/x-ui"
install -m 755 x-ui.sh /usr/bin/ml
[[ -n "$remote_commit" ]] && echo "$remote_commit" > "$COMMIT_FILE"

log "重启面板..."
if ! systemctl restart x-ui; then
    rollback
    fail "面板重启失败，已回滚。请查看：journalctl -u x-ui -e --no-pager"
fi

sleep 2
if ! systemctl is-active --quiet x-ui; then
    rollback
    fail "面板启动后状态异常，已回滚。请查看：journalctl -u x-ui -e --no-pager"
fi

log "更新完成。已保留面板数据、账号密码、安全设置、Xray 配置和 OpenVPN。"

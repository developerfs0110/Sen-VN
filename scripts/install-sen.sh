#!/usr/bin/env sh
# Trình cài đặt và cập nhật CLI `sen` v1.2.0 + SSDS v1.0.0 cho macOS và Linux kèm xác thực SHA-256 qua HTTPS (UTF-8 không BOM, chuẩn LF).
set -eu

REPO="developerfs0110/Sen-VN"
SOURCE=""
EXPECTED_SHA256=""
INSTALL_DIR="${HOME}/.local/bin"
WORKSPACE=""
PACKS=""
SKIP_COMPANIONS="0"
UPDATE_MODE="0"
CONFIGURE_HOOKS="1"
HOOKS_CONFIG="${HOME}/.gemini/config/hooks.json"

while [ "$#" -gt 0 ]; do
    case "$1" in
        --repo)            REPO="$2"; shift 2 ;;
        --source)          SOURCE="$2"; shift 2 ;;
        --sha256)          EXPECTED_SHA256="$2"; shift 2 ;;
        --install-dir)     INSTALL_DIR="$2"; shift 2 ;;
        --workspace)       WORKSPACE="$2"; shift 2 ;;
        --packs)           PACKS="$2"; shift 2 ;;
        --skip-companions) SKIP_COMPANIONS="1"; shift 1 ;;
        --update)          UPDATE_MODE="1"; shift 1 ;;
        --no-hooks)        CONFIGURE_HOOKS="0"; shift 1 ;;
        --hooks-config)    HOOKS_CONFIG="$2"; shift 2 ;;
        *) echo "Unknown flag: $1" >&2; exit 1 ;;
    esac
done

# Từ chối URL HTTP không mã hóa và bắt buộc truyền --sha256 nếu tải từ URL HTTPS tùy chỉnh.
case "$SOURCE" in
    http://*)
        echo "Insecure HTTP URL is forbidden ($SOURCE); use https:// with --sha256." >&2
        exit 1
        ;;
    https://*)
        if [ -z "$EXPECTED_SHA256" ]; then
            echo "--sha256 is required when downloading from a direct HTTPS URL ($SOURCE)." >&2
            exit 1
        fi
        ;;
esac

mkdir -p "$INSTALL_DIR"
TMP_SUM="$(mktemp)"
TMP_BIN="$(mktemp)"
cleanup() { rm -f "$TMP_SUM" "$TMP_BIN"; }
trap cleanup EXIT

# Tính mã băm SHA-256 tương thích cả Linux (sha256sum) và macOS (shasum -a 256).
compute_sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{ print tolower($1) }'
    else
        shasum -a 256 "$1" | awk '{ print tolower($1) }'
    fi
}

# Hợp nhất khóa "sen-vn" vào ~/.gemini/config/hooks.json mà không xóa các khóa hook sẵn có khác.
merge_antigravity_hooks() {
    cfg_path="$1"
    sen_exe="$2"
    mkdir -p "$(dirname "$cfg_path")"
    if command -v python3 >/dev/null 2>&1; then
        python3 - "$cfg_path" "$sen_exe" <<'PY'
import json, os, sys
cfg_path, sen_exe = sys.argv[1], sys.argv[2]
data = {}
if os.path.isfile(cfg_path):
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            raw = f.read().strip()
            if raw:
                data = json.loads(raw)
    except Exception:
        data = {}
q = f'"{sen_exe}"' if " " in sen_exe else sen_exe
data.pop("sen-mvp", None)
data["sen-vn"] = {
    "SessionStart": [{"type": "command", "command": f"{q} brief --hook SessionStart"}],
    "PreInvocation": [{"type": "command", "command": f"{q} brief --hook PreInvocation"}],
    "PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": f"{q} brief --hook PreToolUse"}]}],
    "Stop": [{"type": "command", "command": f"{q} brief --hook Stop"}],
}
with open(cfg_path, "w", encoding="utf-8") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
PY
    elif command -v jq >/dev/null 2>&1 && [ -f "$cfg_path" ]; then
        tmp_json="$(mktemp)"
        case "$sen_exe" in
            *" "*) bin_cmd="\"$sen_exe\"" ;;
            *)     bin_cmd="$sen_exe" ;;
        esac
        jq --arg bin "$bin_cmd" 'del(.["sen-mvp"]) | .["sen-vn"] = {
          "SessionStart": [{"type":"command","command":($bin + " brief --hook SessionStart")}],
          "PreInvocation": [{"type":"command","command":($bin + " brief --hook PreInvocation")}],
          "PreToolUse": [{"matcher":"*","hooks":[{"type":"command","command":($bin + " brief --hook PreToolUse")}]}],
          "Stop": [{"type":"command","command":($bin + " brief --hook Stop")}]
        }' "$cfg_path" > "$tmp_json" && mv "$tmp_json" "$cfg_path"
    elif [ ! -f "$cfg_path" ]; then
        case "$sen_exe" in
            *" "*) bin_cmd="\"$sen_exe\"" ;;
            *)     bin_cmd="$sen_exe" ;;
        esac
        cat > "$cfg_path" <<EOF
{
  "sen-vn": {
    "SessionStart": [{"type": "command", "command": "$bin_cmd brief --hook SessionStart"}],
    "PreInvocation": [{"type": "command", "command": "$bin_cmd brief --hook PreInvocation"}],
    "PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "$bin_cmd brief --hook PreToolUse"}]}],
    "Stop": [{"type": "command", "command": "$bin_cmd brief --hook Stop"}]
  }
}
EOF
    else
        echo "WARNING: Could not merge 'sen-vn' into existing $cfg_path (neither python3 nor jq is installed); existing file left untouched." >&2
        return 0
    fi
    echo "Registered Antigravity lifecycle hooks ('sen-vn') in: $cfg_path"
}

if [ -z "$SOURCE" ]; then
    # Tự động nhận diện hệ điều hành và kiến trúc CPU để tải đúng tệp thực thi từ GitHub Releases.
    UNAME_S="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$UNAME_S" in
        darwin*) OS="darwin" ;;
        linux*)  OS="linux" ;;
        *) echo "Unsupported OS: $UNAME_S" >&2; exit 1 ;;
    esac
    UNAME_M="$(uname -m)"
    case "$UNAME_M" in
        x86_64|amd64)  ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *) echo "Unsupported arch: $UNAME_M" >&2; exit 1 ;;
    esac
    ASSET="sen-${OS}-${ARCH}"
    BASE_URL="https://github.com/${REPO}/releases/latest/download"
    curl -fsSL "$BASE_URL/checksums.txt" -o "$TMP_SUM"
    curl -fsSL "$BASE_URL/$ASSET" -o "$TMP_BIN"
    EXPECTED_SHA256="$(awk -v a="$ASSET" '$2 == a || $2 == "*"a { print tolower($1); exit }' "$TMP_SUM")"
    if [ -z "$EXPECTED_SHA256" ]; then
        echo "Missing SHA-256 entry for $ASSET in checksums.txt" >&2
        exit 1
    fi
    SRC_FILE="$TMP_BIN"
elif printf '%s' "$SOURCE" | grep -q '^https://'; then
    curl -fsSL "$SOURCE" -o "$TMP_BIN"
    SRC_FILE="$TMP_BIN"
else
    SRC_FILE="$SOURCE"
fi

# Đối chiếu mã băm SHA-256 trước khi cấp quyền thực thi tại thư mục đích.
ACTUAL_SHA256="$(compute_sha256 "$SRC_FILE")"
if [ -n "$EXPECTED_SHA256" ]; then
    WANT_LOWER="$(printf '%s' "$EXPECTED_SHA256" | tr '[:upper:]' '[:lower:]')"
    if [ "$ACTUAL_SHA256" != "$WANT_LOWER" ]; then
        echo "SHA-256 verification FAILED! Expected $WANT_LOWER, got $ACTUAL_SHA256" >&2
        exit 1
    fi
    echo "SHA-256 verified: $ACTUAL_SHA256"
fi

cp "$SRC_FILE" "$INSTALL_DIR/sen"
chmod +x "$INSTALL_DIR/sen"
echo "Installed sen successfully at: $INSTALL_DIR/sen"

if [ "$CONFIGURE_HOOKS" = "1" ] && [ -n "$HOOKS_CONFIG" ]; then
    merge_antigravity_hooks "$HOOKS_CONFIG" "$INSTALL_DIR/sen"
fi

# Khởi tạo hoặc đồng bộ ngay không gian làm việc cùng các gói kỹ năng nếu người dùng chỉ định --workspace.
if [ -n "$WORKSPACE" ]; then
    mkdir -p "$WORKSPACE"
    if [ "$UPDATE_MODE" = "1" ] && [ -f "$WORKSPACE/.sen/state.db" ] && [ -z "$PACKS" ]; then
        (cd "$WORKSPACE" && "$INSTALL_DIR/sen" init --update)
    else
        set -- init
        if [ "$SKIP_COMPANIONS" = "0" ]; then
            set -- "$@" --install-companions
        fi
        if [ -n "$PACKS" ]; then
            set -- "$@" --packs "$PACKS"
        fi
        if [ "$UPDATE_MODE" = "1" ]; then
            set -- "$@" --update
        fi
        (cd "$WORKSPACE" && "$INSTALL_DIR/sen" "$@")
    fi
fi

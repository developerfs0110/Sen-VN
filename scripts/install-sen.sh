#!/usr/bin/env sh
# Trình cài đặt CLI `sen` cho macOS và Linux kèm xác thực chữ ký SHA-256 qua HTTPS (UTF-8 không BOM, chuẩn LF).
set -eu

REPO="developerfs0110/Sen-VN"
SOURCE=""
EXPECTED_SHA256=""
INSTALL_DIR="${HOME}/.local/bin"

while [ "$#" -gt 0 ]; do
    case "$1" in
        --repo)        REPO="$2"; shift 2 ;;
        --source)      SOURCE="$2"; shift 2 ;;
        --sha256)      EXPECTED_SHA256="$2"; shift 2 ;;
        --install-dir) INSTALL_DIR="$2"; shift 2 ;;
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

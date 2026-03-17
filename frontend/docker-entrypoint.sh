#!/bin/sh
set -eu

escape_js_string() {
  printf '%s' "${1:-}" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

api_base_url="$(escape_js_string "${VITE_API_BASE_URL:-}")"

cat >/usr/share/nginx/html/runtime-config.js <<EOF
window.__APP_CONFIG__ = {
  VITE_API_BASE_URL: "${api_base_url}"
};
EOF

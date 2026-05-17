#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${TLD_BINARY:-}" ]]; then
  echo "TLD_BINARY is required" >&2
  exit 1
fi

if [[ -z "${TLD_EXPECT_VERSION:-}" ]]; then
  echo "TLD_EXPECT_VERSION is required" >&2
  exit 1
fi

if [[ ! -f "${TLD_BINARY}" ]]; then
  echo "TLD_BINARY does not exist: ${TLD_BINARY}" >&2
  exit 1
fi

expect_version="${TLD_EXPECT_VERSION#v}"

tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/tld-smoke.XXXXXX")"
server_pid=""

cleanup() {
  if [[ -n "${server_pid}" ]]; then
    kill "${server_pid}" >/dev/null 2>&1 || true
    wait "${server_pid}" >/dev/null 2>&1 || true
  fi
  rm -rf "${tmp_root}"
}
trap cleanup EXIT

parse_json_file() {
  node -e '
    const fs = require("fs");
    const payload = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
    if (payload.status && payload.status !== "ok") {
      throw new Error(`unexpected status: ${payload.status}`);
    }
  ' "$1"
}

version_output="$("${TLD_BINARY}" version)"
if [[ "${version_output}" != *"tld version ${expect_version}"* ]]; then
  echo "unexpected version output: ${version_output}" >&2
  echo "expected version: ${expect_version}" >&2
  exit 1
fi

"${TLD_BINARY}" --help >/dev/null

project_dir="${tmp_root}/project"
config_dir="${tmp_root}/config"
data_dir="${tmp_root}/data"
mkdir -p "${project_dir}" "${config_dir}" "${data_dir}"

export TLD_CONFIG_DIR="${config_dir}"
export TLD_DATA_DIR="${data_dir}"

(
  cd "${project_dir}"

  "${TLD_BINARY}" init .tld
  "${TLD_BINARY}" --workspace .tld --format json add "Smoke API" --ref smoke-api --kind service --technology Go > add-api.json
  "${TLD_BINARY}" --workspace .tld --format json add "Smoke DB" --ref smoke-db --kind database --technology SQLite > add-db.json
  "${TLD_BINARY}" --workspace .tld --format json connect --from smoke-api --to smoke-db --label stores > connect.json
  "${TLD_BINARY}" --workspace .tld validate
  "${TLD_BINARY}" --workspace .tld --format json views > views.json

  parse_json_file add-api.json
  parse_json_file add-db.json
  parse_json_file connect.json
  parse_json_file views.json
)

port="$(
  node -e '
    const net = require("net");
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      console.log(server.address().port);
      server.close();
    });
  '
)"

"${TLD_BINARY}" serve --foreground --host 127.0.0.1 --port "${port}" --data-dir "${data_dir}/serve" > "${tmp_root}/serve.log" 2>&1 &
server_pid="$!"

node -e '
  const url = process.argv[1];
  const deadline = Date.now() + 30000;

  async function waitReady() {
    let lastError;
    while (Date.now() < deadline) {
      try {
        const response = await fetch(url);
        const payload = await response.json();
        if (response.ok && payload.ok === true) {
          return;
        }
        lastError = new Error(`status ${response.status}: ${JSON.stringify(payload)}`);
      } catch (error) {
        lastError = error;
      }
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw lastError || new Error("server did not become ready");
  }

  waitReady().catch((error) => {
    console.error(error.message);
    process.exit(1);
  });
' "http://127.0.0.1:${port}/api/ready"

echo "binary smoke test passed"
